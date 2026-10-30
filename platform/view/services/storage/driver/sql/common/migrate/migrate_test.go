/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package migrate_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

// store is a Versioned test store whose steps each create one table.
type store struct {
	name  string
	steps []migrate.Step
}

func (s *store) StoreName() string     { return s.name }
func (s *store) Steps() []migrate.Step { return s.steps }
func (*store) CreateSchema() error {
	return errors.New("CreateSchema must not be called on a Versioned store")
}

func newStore(name string, n int) *store {
	s := &store{name: name}
	for i := 1; i <= n; i++ {
		s.steps = append(s.steps, migrate.Step{
			Version: i,
			Name:    fmt.Sprintf("create %s_t%d", name, i),
			SQL:     []string{fmt.Sprintf("CREATE TABLE %s_t%d (id INT)", name, i)},
		})
	}
	return s
}

// otherStore has the same StoreName as a store but a different type.
type otherStore struct{ *store }

type backend struct {
	dialect migrate.Dialect
	// open returns a new pool on the same database each call.
	open func(t *testing.T) *sql.DB
}

func sqliteBackend(t *testing.T) backend {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "m.db") + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	return backend{dialect: migrate.SQLite, open: func(t *testing.T) *sql.DB {
		t.Helper()
		db, err := sql.Open("sqlite", dsn)
		require.NoError(t, err)
		db.SetMaxOpenConns(1) // the write pool the drivers pass
		t.Cleanup(func() { _ = db.Close() })
		return db
	}}
}

func postgresBackend(t *testing.T) backend {
	t.Helper()
	terminate, dsn, err := postgres.StartPostgres(t.Context(), postgres.ConfigFromEnv(), nil)
	require.NoError(t, err)
	t.Cleanup(terminate)
	return backend{dialect: migrate.Postgres, open: func(t *testing.T) *sql.DB {
		t.Helper()
		db, err := sql.Open("pgx", dsn)
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		return db
	}}
}

func TestMigrateSQLite(t *testing.T) {
	t.Parallel()
	testMigrate(t, sqliteBackend)
}

func TestPostgresMigrate(t *testing.T) {
	t.Parallel()
	b := postgresBackend(t)
	testMigrate(t, func(*testing.T) backend { return b })

	t.Run("NothingPendingSkipsLock", func(t *testing.T) {
		t.Parallel()
		db := b.open(t)
		ctx := t.Context()
		s := newStore("nolock_s", 1)
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "nolock", s))

		holder, err := db.Conn(ctx)
		require.NoError(t, err)
		defer func() { _ = holder.Close() }()
		_, err = holder.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrate.LockKey("nolock_schema_versions"))
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "nolock", s))
	})
}

// TestPostgresMigrateWaiter checks that a migrator waits for the advisory lock,
// logs while waiting, and proceeds once the lock is released. It is not
// parallel because it replaces the package logger and lock intervals, and
// sequential top-level tests finish before parallel ones start.
func TestPostgresMigrateWaiter(t *testing.T) { //nolint:paralleltest // replaces package-level state
	b := postgresBackend(t)
	migrate.SetLockIntervals(t, 10*time.Millisecond, 50*time.Millisecond)
	logs := migrate.RecordLogs(t)
	db := b.open(t)
	ctx := t.Context()

	holder, err := db.Conn(ctx)
	require.NoError(t, err)
	defer func() { _ = holder.Close() }()
	_, err = holder.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrate.LockKey("waiter_schema_versions"))
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- migrate.Migrate(ctx, db, b.dialect, "waiter", newStore("waiter_s", 1)) }()
	require.Eventually(t, func() bool {
		return len(logs.EntriesContaining("waiting")) > 0
	}, 5*time.Second, 10*time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("migrator proceeded while the lock was held: %v", err)
	default:
	}

	_, err = holder.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrate.LockKey("waiter_schema_versions"))
	require.NoError(t, err)
	require.NoError(t, <-done)
	requireVersion(t, db, "waiter_schema_versions", "waiter_s", 1)
}

func testMigrate(t *testing.T, newBackend func(*testing.T) backend) {
	t.Helper()

	t.Run("StepsRunInOrderOnce", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		s := &store{name: "order_s", steps: []migrate.Step{
			{Version: 1, Name: "create", SQL: []string{"CREATE TABLE order_s (id INT)"}},
			{Version: 2, Name: "insert", SQL: []string{"INSERT INTO order_s (id) VALUES (1)"}},
		}}
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "order", s))
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "order", s))
		var n int
		require.NoError(t, db.QueryRow("SELECT count(*) FROM order_s").Scan(&n))
		require.Equal(t, 1, n)
		requireVersion(t, db, "order_schema_versions", "order_s", 2)

		// A later binary adds a step; only that step runs.
		s.steps = append(s.steps, migrate.Step{Version: 3, Name: "insert again", SQL: []string{"INSERT INTO order_s (id) VALUES (2)"}})
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "order", s))
		require.NoError(t, db.QueryRow("SELECT count(*) FROM order_s").Scan(&n))
		require.Equal(t, 2, n)
		requireVersion(t, db, "order_schema_versions", "order_s", 3)
	})

	t.Run("FailedStepLeavesRow", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		s := newStore("fail_s", 1)
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "fail", s))
		s.steps = append(s.steps, migrate.Step{Version: 2, Name: "broken", SQL: []string{
			"CREATE TABLE fail_partial (id INT)",
			"NOT SQL",
		}})
		err := migrate.Migrate(t.Context(), db, b.dialect, "fail", s)
		require.ErrorContains(t, err, "fail_s")
		require.ErrorContains(t, err, "broken")
		requireVersion(t, db, "fail_schema_versions", "fail_s", 1)
		requireNoTable(t, db, b.dialect, "fail_partial")

		// The pool is usable afterwards; on SQLite it has a single connection.
		s.steps[1].SQL = []string{"CREATE TABLE fail_fixed (id INT)"}
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "fail", s))
		requireVersion(t, db, "fail_schema_versions", "fail_s", 2)
	})

	t.Run("ConcurrentMigratorsApplyOnce", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		s := &store{name: "conc_s", steps: []migrate.Step{
			{Version: 1, Name: "create", SQL: []string{"CREATE TABLE conc_s (id INT)"}},
			{Version: 2, Name: "insert", SQL: []string{"INSERT INTO conc_s (id) VALUES (1)"}},
		}}
		const n = 4
		dbs := make([]*sql.DB, n)
		for i := range dbs {
			dbs[i] = b.open(t)
		}
		var wg sync.WaitGroup
		errs := make([]error, n)
		for i := range n {
			wg.Go(func() { errs[i] = migrate.Migrate(t.Context(), dbs[i], b.dialect, "conc", s) })
		}
		wg.Wait()
		for _, err := range errs {
			require.NoError(t, err)
		}
		var rows int
		require.NoError(t, dbs[0].QueryRow("SELECT count(*) FROM conc_s").Scan(&rows))
		require.Equal(t, 1, rows)
		requireVersion(t, dbs[0], "conc_schema_versions", "conc_s", 2)
	})

	t.Run("SchemaTooNew", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		s := newStore("new_s", 2)
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "new", s))
		_, err := db.Exec("UPDATE new_schema_versions SET version = 3, readable_by = 3 WHERE store = 'new_s'")
		require.NoError(t, err)
		err = migrate.Migrate(t.Context(), db, b.dialect, "new", s)
		require.ErrorIs(t, err, migrate.ErrSchemaTooNew)
		require.ErrorContains(t, err, "new_s")
		require.ErrorContains(t, err, "needs version 3, binary knows 2")

		// A binary that is behind but can still read the store opens it.
		_, err = db.Exec("UPDATE new_schema_versions SET version = 3, readable_by = 2 WHERE store = 'new_s'")
		require.NoError(t, err)
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "new", s))

		// A corrupt row is an error, not a panic.
		_, err = db.Exec("UPDATE new_schema_versions SET version = -1, readable_by = 0 WHERE store = 'new_s'")
		require.NoError(t, err)
		require.ErrorContains(t, migrate.Migrate(t.Context(), db, b.dialect, "new", s), "invalid row")
	})

	t.Run("CheckOnly", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		s := newStore("check_s", 1)
		require.ErrorIs(t, migrate.Migrate(t.Context(), db, b.dialect, "check", s, migrate.WithCheckOnly()), migrate.ErrPending)
		requireNoTable(t, db, b.dialect, "check_schema_versions")
		requireNoTable(t, db, b.dialect, "check_s_t1")

		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "check", s))
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "check", s, migrate.WithCheckOnly()))
	})

	t.Run("DuplicateStoreName", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		s := newStore("dup_s", 1)
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "dup", s))
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "dup", newStore("dup_s", 1)), "same type reopens")
		require.ErrorContains(t, migrate.Migrate(t.Context(), db, b.dialect, "dup", otherStore{s}), "used by both")
		require.NoError(t, migrate.Migrate(t.Context(), b.open(t), b.dialect, "dup", otherStore{s}), "another pool is another database")
	})

	t.Run("MixedCasePrefix", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "MiXed", newStore("mixed_s", 1)))
		// Both backends match unquoted names case-insensitively, so check the catalog.
		q := "SELECT name FROM sqlite_master WHERE lower(name) = 'mixed_schema_versions'"
		if b.dialect == migrate.Postgres {
			q = "SELECT tablename FROM pg_tables WHERE lower(tablename) = 'mixed_schema_versions'"
		}
		var name string
		require.NoError(t, db.QueryRow(q).Scan(&name))
		require.Equal(t, "mixed_schema_versions", name)
	})

	t.Run("StepsMustBeNumbered", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		s := &store{name: "num_s", steps: []migrate.Step{{Version: 2, Name: "skip one"}}}
		require.ErrorContains(t, migrate.Migrate(t.Context(), b.open(t), b.dialect, "num", s), "expected 1")
	})
}

func requireVersion(t *testing.T, db *sql.DB, table, store string, want int) {
	t.Helper()
	var version, readableBy int
	require.NoError(t, db.QueryRow("SELECT version, readable_by FROM "+table+" WHERE store = $1", store).Scan(&version, &readableBy))
	require.Equal(t, want, version)
	require.Equal(t, want, readableBy)
}

func requireNoTable(t *testing.T, db *sql.DB, d migrate.Dialect, table string) {
	t.Helper()
	q := "SELECT count(*) FROM sqlite_master WHERE name = $1"
	if d == migrate.Postgres {
		q = "SELECT count(*) FROM pg_tables WHERE tablename = $1"
	}
	var n int
	require.NoError(t, db.QueryRow(q, table).Scan(&n))
	require.Zero(t, n, "table [%s] exists", table)
}
