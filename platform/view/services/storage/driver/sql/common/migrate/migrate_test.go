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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
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

// plainStore is a DBObject that does not implement Versioned.
type plainStore struct{ calls int }

func (p *plainStore) CreateSchema() error { p.calls++; return nil }

type backend struct {
	dialect migrate.Dialect
	// dsn is the Postgres data source; empty on SQLite.
	dsn string
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
		// The drivers ping every pool on open, which applies the pragmas
		// before concurrent use; switching a new file to WAL can fail with
		// SQLITE_BUSY without waiting for busy_timeout.
		require.NoError(t, db.Ping())
		t.Cleanup(func() { _ = db.Close() })
		return db
	}}
}

func postgresBackend(t *testing.T) backend {
	t.Helper()
	terminate, dsn, err := postgres.StartPostgres(t.Context(), postgres.ConfigFromEnv(), nil)
	require.NoError(t, err)
	t.Cleanup(terminate)
	return backend{dialect: migrate.Postgres, dsn: dsn, open: func(t *testing.T) *sql.DB {
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

	holder, pid := lockHolder(t, db, "waiter_schema_versions")

	done := make(chan error, 1)
	go func() { done <- migrate.Migrate(ctx, db, b.dialect, "waiter", newStore("waiter_s", 1)) }()
	require.Eventually(t, func() bool {
		return len(logs.EntriesContaining(fmt.Sprintf("held by pid %d (application_name [holder-pod-0]", pid))) > 0
	}, 5*time.Second, 10*time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("migrator proceeded while the lock was held: %v", err)
	default:
	}

	_, err := holder.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrate.LockKey("waiter_schema_versions"))
	require.NoError(t, err)
	require.NoError(t, <-done)
	requireVersion(t, db, "waiter_schema_versions", "waiter_s", 1)
}

// TestPostgresMigrateLockTimeout checks that a migrator stops waiting after
// WithLockTimeout with an error naming the holder and the table.
func TestPostgresMigrateLockTimeout(t *testing.T) { //nolint:paralleltest // replaces package-level state
	b := postgresBackend(t)
	migrate.SetLockIntervals(t, 10*time.Millisecond, time.Hour)
	db := b.open(t)
	_, pid := lockHolder(t, db, "timeout_schema_versions")

	err := migrate.Migrate(t.Context(), db, b.dialect, "timeout", newStore("timeout_s", 1), migrate.WithLockTimeout(200*time.Millisecond))
	require.ErrorContains(t, err, "migration lock on [timeout_schema_versions] not granted within 200ms")
	require.ErrorContains(t, err, fmt.Sprintf("held by pid %d (application_name [holder-pod-0]", pid))
	requireNoTable(t, db, b.dialect, "timeout_s_t1")
}

// TestPostgresOpenerPrivilege checks that a role that may not write the openers
// table fails to open a store, with an error naming the table, and that the
// grant the docs give the node role is enough.
func TestPostgresOpenerPrivilege(t *testing.T) {
	t.Parallel()
	b := postgresBackend(t)
	db := b.open(t)
	s := newStore("priv_s", 1)
	require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "priv", s))
	exec := func(q string) {
		t.Helper()
		_, err := db.Exec(q)
		require.NoError(t, err)
	}
	exec("CREATE ROLE priv_node LOGIN PASSWORD 'p'")
	t.Cleanup(func() {
		_, _ = db.Exec("DROP OWNED BY priv_node")
		_, _ = db.Exec("DROP ROLE priv_node")
	})
	exec("GRANT SELECT ON priv_schema_versions TO priv_node")
	exec("GRANT INSERT, UPDATE ON priv_schema_openers TO priv_node")
	cfg, err := pgx.ParseConfig(b.dsn)
	require.NoError(t, err)
	cfg.User, cfg.Password = "priv_node", "p"
	nodeDB := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { _ = nodeDB.Close() })
	err = migrate.Migrate(t.Context(), nodeDB, b.dialect, "priv", s, migrate.WithCheckOnly())
	require.ErrorContains(t, err, "failed to record opener in [priv_schema_openers]")
	require.ErrorContains(t, err, "permission denied")

	exec("GRANT SELECT ON priv_schema_openers TO priv_node")
	require.NoError(t, migrate.Migrate(t.Context(), nodeDB, b.dialect, "priv", s, migrate.WithCheckOnly()))
}

// lockHolder takes the advisory lock of table on a connection named
// holder-pod-0 and returns the connection and its backend pid.
func lockHolder(t *testing.T, db *sql.DB, table string) (*sql.Conn, int) {
	t.Helper()
	holder, err := db.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Close() })
	_, err = holder.ExecContext(t.Context(), "SET application_name = 'holder-pod-0'")
	require.NoError(t, err)
	var pid int
	require.NoError(t, holder.QueryRowContext(t.Context(), "SELECT pg_backend_pid()").Scan(&pid))
	_, err = holder.ExecContext(t.Context(), "SELECT pg_advisory_lock($1)", migrate.LockKey(table))
	require.NoError(t, err)
	return holder, pid
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
		s := newStore("check_s", 2)
		err := migrate.Migrate(t.Context(), db, b.dialect, "check", s, migrate.WithCheckOnly())
		require.ErrorIs(t, err, migrate.ErrPending)
		require.ErrorContains(t, err, "pending: step 1 [create check_s_t1], step 2 [create check_s_t2]")
		requireNoTable(t, db, b.dialect, "check_schema_versions")
		requireNoTable(t, db, b.dialect, "check_schema_openers")
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

	t.Run("GoStep", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		backfill := func(ctx context.Context, q migrate.Execer) error {
			ids, err := readIDs(ctx, q)
			if err != nil {
				return err
			}
			for _, id := range ids {
				if _, err := q.ExecContext(ctx, "UPDATE go_s SET sq = $1 WHERE id = $2", id*id, id); err != nil {
					return err
				}
			}
			return nil
		}
		s := &store{name: "go_s", steps: []migrate.Step{
			{Version: 1, Name: "create", SQL: []string{"CREATE TABLE go_s (id INT)", "INSERT INTO go_s (id) VALUES (2), (3)"}},
			{Version: 2, Name: "add sq", SQL: []string{"ALTER TABLE go_s ADD COLUMN sq INT"}, Go: func(context.Context, migrate.Execer) error {
				return errors.New("backfill failed")
			}},
		}}
		err := migrate.Migrate(t.Context(), db, b.dialect, "go", s)
		require.ErrorContains(t, err, "backfill failed")
		require.ErrorContains(t, err, "add sq")
		requireVersion(t, db, "go_schema_versions", "go_s", 1)
		_, err = db.Exec("SELECT sq FROM go_s")
		require.Error(t, err, "the step's SQL rolled back with its Go function")

		s.steps[1].Go = backfill
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "go", s))
		requireVersion(t, db, "go_schema_versions", "go_s", 2)
		var sum int
		require.NoError(t, db.QueryRow("SELECT sum(sq) FROM go_s").Scan(&sum))
		require.Equal(t, 13, sum)
	})

	t.Run("CreateSchemaCheckOnly", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		v := newStore("cso_s", 1)
		require.ErrorIs(t, migrate.CreateSchema(t.Context(), db, b.dialect, "cso", v, migrate.WithCheckOnly()), migrate.ErrPending)
		requireNoTable(t, db, b.dialect, "cso_s_t1")
		require.NoError(t, migrate.CreateSchema(t.Context(), db, b.dialect, "cso", v))
		require.NoError(t, migrate.CreateSchema(t.Context(), db, b.dialect, "cso", v, migrate.WithCheckOnly()))

		p := &plainStore{}
		require.NoError(t, migrate.CreateSchema(t.Context(), db, b.dialect, "cso", p, migrate.WithCheckOnly()))
		require.Zero(t, p.calls)
		require.NoError(t, migrate.CreateSchema(t.Context(), db, b.dialect, "cso", p))
		require.Equal(t, 1, p.calls)
	})

	t.Run("Openers", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		s := newStore("open_s", 1)
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "open", s, migrate.WithNode("alice")))
		requireOpener(t, db, "open_schema_openers", "open_s", "alice", 1)

		// Reopening updates the row; a check-only open records too.
		_, err := db.Exec("UPDATE open_schema_openers SET opened_at = '2000-01-01 00:00:00'")
		require.NoError(t, err)
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "open", s, migrate.WithNode("alice"), migrate.WithCheckOnly()))
		require.Greater(t, requireOpener(t, db, "open_schema_openers", "open_s", "alice", 1).Year(), 2000)

		// A replica on a newer binary gets its own row; the older row stays.
		s.steps = append(s.steps, newStore("open_s", 2).steps[1])
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "open", s, migrate.WithNode("alice")))
		requireOpener(t, db, "open_schema_openers", "open_s", "alice", 1)
		requireOpener(t, db, "open_schema_openers", "open_s", "alice", 2)
		var n int
		require.NoError(t, db.QueryRow("SELECT count(*) FROM open_schema_openers").Scan(&n))
		require.Equal(t, 2, n)
	})

	t.Run("StepsMustBeNumbered", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		s := &store{name: "num_s", steps: []migrate.Step{{Version: 2, Name: "skip one"}}}
		require.ErrorContains(t, migrate.Migrate(t.Context(), b.open(t), b.dialect, "num", s), "expected 1")
	})
}

// readIDs reads go_s.id, closing the rows before the caller writes: SQLite
// cannot update a table while a query on it is open on the same connection.
func readIDs(ctx context.Context, q migrate.Execer) ([]int, error) {
	rows, err := q.QueryContext(ctx, "SELECT id FROM go_s")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func requireVersion(t *testing.T, db *sql.DB, table, store string, want int) {
	t.Helper()
	var version, readableBy int
	require.NoError(t, db.QueryRow("SELECT version, readable_by FROM "+table+" WHERE store = $1", store).Scan(&version, &readableBy))
	require.Equal(t, want, version)
	require.Equal(t, want, readableBy)
}

// requireOpener returns opened_at of the opener row, failing if it is missing.
func requireOpener(t *testing.T, db *sql.DB, table, store, node string, highest int) time.Time {
	t.Helper()
	var at time.Time
	require.NoError(t, db.QueryRow("SELECT opened_at FROM "+table+" WHERE store = $1 AND node = $2 AND highest_step = $3", store, node, highest).Scan(&at))
	return at
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

// InProgress is true while a step runs and false once Migrate returns, also
// after a failed step. Go runs a test without t.Parallel before the parallel
// ones, so no other migrator of the package runs meanwhile.
func TestInProgress(t *testing.T) { //nolint:paralleltest // reads package-level state
	db := sqliteBackend(t).open(t)
	require.False(t, migrate.InProgress())

	for _, fail := range []bool{false, true} {
		running, release := make(chan struct{}), make(chan struct{})
		s := &store{name: fmt.Sprintf("progress_%v", fail), steps: []migrate.Step{{
			Version: 1, Name: "block",
			Go: func(context.Context, migrate.Execer) error {
				close(running)
				<-release
				if fail {
					return errors.New("step failed")
				}
				return nil
			},
		}}}
		done := make(chan error, 1)
		go func() { done <- migrate.Migrate(t.Context(), db, migrate.SQLite, "", s) }()
		<-running
		require.True(t, migrate.InProgress(), "while the step runs")
		close(release)
		err := <-done
		if fail {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
		require.False(t, migrate.InProgress(), "after Migrate returns")
	}

	// A store with nothing pending and a check-only call never count; neither waits.
	require.NoError(t, migrate.Migrate(t.Context(), db, migrate.SQLite, "", newStore("progress_done", 1)))
	require.NoError(t, migrate.Migrate(t.Context(), db, migrate.SQLite, "", newStore("progress_done", 1)))
	require.ErrorIs(t, migrate.Migrate(t.Context(), db, migrate.SQLite, "", newStore("progress_check", 1), migrate.WithCheckOnly()), migrate.ErrPending)
	require.False(t, migrate.InProgress())
}
