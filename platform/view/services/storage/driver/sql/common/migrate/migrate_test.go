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
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

// store is a Versioned test store whose steps each create one table. It also
// implements migrate.ReadableByObserver, recording the readable_by of its
// last successful open. readableBy is an atomic because
// ConcurrentMigratorsApplyOnce shares one store across several goroutines
// that call Migrate concurrently, standing in for separate migrators.
type store struct {
	name       string
	steps      []migrate.Step
	readableBy atomic.Int64
}

func (s *store) StoreName() string     { return s.name }
func (s *store) Steps() []migrate.Step { return s.steps }
func (s *store) SetReadableBy(v int)   { s.readableBy.Store(int64(v)) }
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

	// An older binary that records its opener while a contract step runs waits
	// for the step's lock on the openers table and then refuses the store.
	t.Run("OlderOpenerDuringContractStep", func(t *testing.T) {
		t.Parallel()
		db := b.open(t)
		ctx := t.Context()
		previous := newStore("race_s", 1)
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "race", previous, migrate.WithNode("old")))
		_, err := db.Exec("DELETE FROM race_schema_openers")
		require.NoError(t, err)

		running, release := make(chan struct{}), make(chan struct{})
		next := newStore("race_s", 2)
		next.steps[1].Contract = true
		next.steps[1].Go = func(context.Context, migrate.Execer) error {
			close(running)
			<-release
			return nil
		}
		done := make(chan error, 1)
		go func() { done <- migrate.Migrate(ctx, db, b.dialect, "race", next, migrate.WithContract()) }()
		<-running

		opened := make(chan error, 1)
		go func() { opened <- migrate.Migrate(ctx, db, b.dialect, "race", previous, migrate.WithNode("old")) }()
		select {
		case err := <-opened:
			t.Fatalf("the older binary opened the store during the contract step: %v", err)
		case <-time.After(500 * time.Millisecond):
		}
		close(release)
		require.NoError(t, <-done)
		require.ErrorIs(t, <-opened, migrate.ErrSchemaTooNew)
		// The refused binary left no row that would block a later contract step.
		var n int
		require.NoError(t, db.QueryRow("SELECT count(*) FROM race_schema_openers WHERE node = 'old'").Scan(&n))
		require.Zero(t, n)
	})

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

// TestMigrateFilledMeanwhile and TestPostgresMigrateFilledMeanwhile run
// testFilledMeanwhile. They are not parallel because they wait for InProgress,
// which other migrations of this process would also set.
func TestMigrateFilledMeanwhile(t *testing.T) { //nolint:paralleltest // reads package-level state
	testFilledMeanwhile(t, sqliteBackend(t))
}

func TestPostgresMigrateFilledMeanwhile(t *testing.T) { //nolint:paralleltest // reads package-level state
	migrate.SetLockIntervals(t, 10*time.Millisecond, time.Hour)
	testFilledMeanwhile(t, postgresBackend(t))
}

// testFilledMeanwhile checks that a newer binary which read an empty store
// holds its contract step back when an older binary applies step 1 before the
// newer one takes the lock, even though the older binary has not recorded its
// opener row yet: the older binary then still opens the store. The test plays
// the older binary on its own connection and stops it between its step and
// its opener row, the window ContractStepOnStoreFilledMeanwhile hits only by
// chance.
func testFilledMeanwhile(t *testing.T, b backend) {
	t.Helper()
	ctx := t.Context()
	var holder *sql.Conn
	if b.dialect == migrate.Postgres {
		holder, _ = lockHolder(t, b.open(t), "race_schema_versions")
	} else {
		var err error
		holder, err = b.open(t).Conn(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { _ = holder.Close() })
	}
	exec := func(q string) {
		t.Helper()
		_, err := holder.ExecContext(ctx, q)
		require.NoError(t, err)
	}
	if b.dialect == migrate.SQLite {
		exec("BEGIN IMMEDIATE")
	}

	next := newStore("race_s", 2)
	next.steps[1].Contract = true
	newDB := b.open(t)
	newDone := make(chan error, 1)
	go func() {
		newDone <- migrate.Migrate(ctx, newDB, b.dialect, "race", next, migrate.WithNode("new"))
	}()
	// InProgress turns true once the newer binary has read the empty store
	// and goes for the lock the older binary holds.
	require.Eventually(t, migrate.InProgress, 5*time.Second, time.Millisecond)

	if b.dialect == migrate.Postgres {
		exec("BEGIN")
	}
	exec("CREATE TABLE race_schema_versions (store TEXT PRIMARY KEY, version INT NOT NULL, readable_by INT NOT NULL)")
	exec("CREATE TABLE race_schema_openers (store TEXT NOT NULL, node TEXT NOT NULL, highest_step INT NOT NULL, opened_at TIMESTAMP NOT NULL, PRIMARY KEY (store, node, highest_step))")
	exec("CREATE TABLE race_s_t1 (id INT)")
	exec("INSERT INTO race_schema_versions (store, version, readable_by) VALUES ('race_s', 1, 0)")
	exec("COMMIT")
	if b.dialect == migrate.Postgres {
		exec("SELECT pg_advisory_unlock(" + strconv.FormatInt(migrate.LockKey("race_schema_versions"), 10) + ")")
	}

	require.NoError(t, <-newDone)
	requireVersion(t, newDB, "race_schema_versions", "race_s", 1)
	requireNoTable(t, newDB, b.dialect, "race_s_t2")
	// The older binary records its opener row now and still reads the store.
	require.NoError(t, migrate.Migrate(ctx, b.open(t), b.dialect, "race", newStore("race_s", 1), migrate.WithNode("old")))
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

	t.Run("PreviousReleaseReadsAfterExpandStep", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		previous := newStore("prev_s", 2)
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "prev", previous))
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "prev", newStore("prev_s", 3)))

		// A replica of the previous release restarts against the migrated store.
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "prev", previous))
	})

	t.Run("ContractStep", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		ctx := t.Context()
		// The previous release writes v in the old column.
		previous := &store{name: "con_s", steps: []migrate.Step{
			{Version: 1, Name: "create", SQL: []string{"CREATE TABLE con_s (id INT, old_v INT)"}},
		}}
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "con", previous, migrate.WithNode("n")))
		_, err := db.Exec("INSERT INTO con_s (id, old_v) VALUES (1, 10)")
		require.NoError(t, err)

		// The next release adds new_v (expand), then drops old_v (contract), then adds an index.
		next := &store{name: "con_s", steps: append(slices.Clone(previous.steps),
			migrate.Step{Version: 2, Name: "add new_v", SQL: []string{"ALTER TABLE con_s ADD COLUMN new_v INT"}},
			migrate.Step{Version: 3, Name: "drop old_v", Contract: true, SQL: []string{
				"UPDATE con_s SET new_v = old_v WHERE new_v IS NULL",
				"ALTER TABLE con_s DROP COLUMN old_v",
			}},
			migrate.Step{Version: 4, Name: "index new_v", SQL: []string{"CREATE INDEX con_s_new_v ON con_s (new_v)"}},
		)}

		// Startup runs the expand step and stops before the contract step.
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "con", next, migrate.WithNode("n")))
		requireRow(t, db, "con_schema_versions", "con_s", 2, 0)
		// Check-only does not count the held-back steps as pending.
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "con", next, migrate.WithNode("n"), migrate.WithCheckOnly()))

		// A replica of the previous release still opens the store and writes the old form.
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "con", previous, migrate.WithNode("n")))
		_, err = db.Exec("INSERT INTO con_s (id, old_v) VALUES (2, 20)")
		require.NoError(t, err)

		// --contract refuses while that binary's opener row exists, and names
		// it; so does --check --contract, without changing anything.
		for _, opts := range [][]migrate.Option{{migrate.WithContract()}, {migrate.WithContract(), migrate.WithCheckOnly()}} {
			err = migrate.Migrate(ctx, db, b.dialect, "con", next, append(opts, migrate.WithNode("n"))...)
			require.ErrorIs(t, err, migrate.ErrOlderOpener)
			require.ErrorContains(t, err, "contract step 3 [drop old_v] is blocked: node [n] knows step 1, last opened ")
			requireRow(t, db, "con_schema_versions", "con_s", 2, 0)
		}
		// Once nothing blocks it, --check --contract lists the contract step as pending.
		_, err = db.Exec("DELETE FROM con_schema_openers WHERE highest_step < 3")
		require.NoError(t, err)
		err = migrate.Migrate(ctx, db, b.dialect, "con", next, migrate.WithNode("n"), migrate.WithCheckOnly(), migrate.WithContract())
		require.ErrorIs(t, err, migrate.ErrPending)
		require.ErrorContains(t, err, "pending: step 3 [drop old_v] (contract), step 4 [index new_v]")
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "con", previous, migrate.WithNode("n")))

		// Forgetting the node lets it run; the backfill keeps both rows.
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "con", next, migrate.WithNode("n"), migrate.WithContract(), migrate.WithForgetOpeners("n")))
		requireRow(t, db, "con_schema_versions", "con_s", 4, 3)
		var sum int
		require.NoError(t, db.QueryRow("SELECT sum(new_v) FROM con_s").Scan(&sum))
		require.Equal(t, 30, sum)
		requireOpener(t, db, "con_schema_openers", "con_s", "n", 4)

		// The previous release now refuses the store before recording itself,
		// and so does a binary with step 2 only.
		require.ErrorIs(t, migrate.Migrate(ctx, db, b.dialect, "con", previous, migrate.WithNode("n")), migrate.ErrSchemaTooNew)
		require.ErrorIs(t, migrate.Migrate(ctx, db, b.dialect, "con", &store{name: "con_s", steps: next.steps[:2]}, migrate.WithNode("n")), migrate.ErrSchemaTooNew)
		var older int
		require.NoError(t, db.QueryRow("SELECT count(*) FROM con_schema_openers WHERE highest_step < 3").Scan(&older))
		require.Zero(t, older)
		// A binary with the contract step but not the later index opens it.
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "con", &store{name: "con_s", steps: next.steps[:3]}))
	})

	t.Run("ContractStepUpgradedOpener", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		ctx := t.Context()
		next := newStore("up_s", 3)
		next.steps[2].Contract = true
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "up", newStore("up_s", 1), migrate.WithNode("a")))
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "up", next, migrate.WithNode("b")))
		requireVersion(t, db, "up_schema_versions", "up_s", 2)

		// Forgetting another node leaves a's row, which blocks.
		err := migrate.Migrate(ctx, db, b.dialect, "up", next, migrate.WithNode("b"), migrate.WithContract(), migrate.WithForgetOpeners("c"))
		require.ErrorIs(t, err, migrate.ErrOlderOpener)
		require.ErrorContains(t, err, "node [a] knows step 1")
		require.NotContains(t, err.Error(), "node [b]")

		// A row of a binary that knows the step does not block.
		_, err = db.Exec("DELETE FROM up_schema_openers WHERE node = 'a'")
		require.NoError(t, err)
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "up", next, migrate.WithNode("a")))
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "up", next, migrate.WithNode("b"), migrate.WithContract()))
		requireRow(t, db, "up_schema_versions", "up_s", 3, 3)
	})

	t.Run("ContractStepReplicasShareNode", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		ctx := t.Context()
		next := newStore("rep_s", 2)
		next.steps[1].Contract = true
		// Replicas share their node's FSC id: one opens on the previous release, then
		// every replica runs the next one.
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "rep", newStore("rep_s", 1), migrate.WithNode("bob")))
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "rep", next, migrate.WithNode("bob")))

		// The previous release's row stays, so the upgrade alone does not unblock the step.
		err := migrate.Migrate(ctx, db, b.dialect, "rep", next, migrate.WithNode("bob"), migrate.WithContract())
		require.ErrorIs(t, err, migrate.ErrOlderOpener)
		require.ErrorContains(t, err, "node [bob] knows step 1")
		require.ErrorContains(t, err, "run node migrate --contract --forget-opener <node>")
		require.NoError(t, migrate.Migrate(ctx, db, b.dialect, "rep", next, migrate.WithNode("bob"), migrate.WithContract(), migrate.WithForgetOpeners("bob")))
		requireRow(t, db, "rep_schema_versions", "rep_s", 2, 2)
		requireOpener(t, db, "rep_schema_openers", "rep_s", "bob", 2)
	})

	t.Run("ContractStepFailureRollsBack", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		s := newStore("cf_s", 1)
		s.steps = append(s.steps, migrate.Step{Version: 2, Name: "broken", Contract: true, SQL: []string{
			"DROP TABLE cf_s_t1",
			"NOT SQL",
		}})
		require.ErrorContains(t, migrate.Migrate(t.Context(), db, b.dialect, "cf", s, migrate.WithContract()), "broken")
		requireRow(t, db, "cf_schema_versions", "cf_s", 1, 0)
		// The table the step dropped is back, and the binary without the step still opens the store.
		_, err := db.Exec("SELECT id FROM cf_s_t1")
		require.NoError(t, err)
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "cf", newStore("cf_s", 1)))
	})

	// An older binary that wins the lock on an empty store and runs step 1
	// makes a newer binary that started at the same time hold its contract
	// step back instead of failing.
	t.Run("ContractStepOnStoreFilledMeanwhile", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		oldDB, newDB := b.open(t), b.open(t)
		running, release := make(chan struct{}), make(chan struct{})
		old := newStore("fill_s", 1)
		old.steps[0].Go = func(context.Context, migrate.Execer) error {
			close(running)
			<-release
			return nil
		}
		next := newStore("fill_s", 2)
		next.steps[1].Contract = true

		oldDone := make(chan error, 1)
		go func() {
			oldDone <- migrate.Migrate(t.Context(), oldDB, b.dialect, "fill", old, migrate.WithNode("old"))
		}()
		<-running
		newDone := make(chan error, 1)
		go func() {
			newDone <- migrate.Migrate(t.Context(), newDB, b.dialect, "fill", next, migrate.WithNode("new"))
		}()
		time.Sleep(200 * time.Millisecond) // the newer binary has read no row and waits for the lock
		close(release)
		require.NoError(t, <-oldDone)
		require.NoError(t, <-newDone)
		requireVersion(t, oldDB, "fill_schema_versions", "fill_s", 1)
	})

	// An older binary that opened a fresh store after its earlier steps makes
	// the contract step wait at startup instead of failing the open.
	t.Run("ContractStepOnFreshStoreOpenedMeanwhile", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		s := newStore("gap_s", 2)
		s.steps[1].Contract = true
		// Step 1 runs in the transaction that creates the store's row; the
		// opener row it writes stands for an older binary that opened the
		// store right after it committed.
		s.steps[0].Go = func(ctx context.Context, q migrate.Execer) error {
			_, err := q.ExecContext(ctx, "INSERT INTO gap_schema_openers (store, node, highest_step, opened_at) VALUES ('gap_s', 'old', 1, CURRENT_TIMESTAMP)")
			return err
		}
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "gap", s, migrate.WithNode("new")))
		requireVersion(t, db, "gap_schema_versions", "gap_s", 1)
	})

	t.Run("ContractStepOnFreshStore", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		s := newStore("fresh_s", 3)
		s.steps[1].Contract = true
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "fresh", s))
		requireRow(t, db, "fresh_schema_versions", "fresh_s", 3, 2)
	})

	// A store without a row whose table exists, as a binary without
	// versioning created it, is not fresh: its contract step waits.
	t.Run("ContractStepOnUnversionedStore", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		s := &store{name: "unv_s", steps: []migrate.Step{
			{Version: 1, Name: "create", SQL: []string{"CREATE TABLE IF NOT EXISTS unv_s (id INT)"}},
			{Version: 2, Name: "drop", Contract: true, SQL: []string{"DROP TABLE unv_s"}},
		}}
		_, err := db.Exec("CREATE TABLE unv_s (id INT)")
		require.NoError(t, err)
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "unv", s))
		requireVersion(t, db, "unv_schema_versions", "unv_s", 1)
		_, err = db.Exec("SELECT id FROM unv_s")
		require.NoError(t, err)
	})

	// --check without --contract lists only the steps node migrate would run.
	t.Run("CheckOnlyListsRunnableSteps", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		s := newStore("lst_s", 4)
		s.steps[2].Contract = true
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "lst", newStore("lst_s", 1)))
		err := migrate.Migrate(t.Context(), db, b.dialect, "lst", s, migrate.WithCheckOnly())
		require.ErrorIs(t, err, migrate.ErrPending)
		require.ErrorContains(t, err, "pending: step 2 [create lst_s_t2]: ")
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

	t.Run("OpenersTableAddedToMigratedDatabase", func(t *testing.T) {
		t.Parallel()
		b := newBackend(t)
		db := b.open(t)
		s := newStore("heal_s", 1)
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "heal", s))
		_, err := db.Exec("DROP TABLE heal_schema_openers")
		require.NoError(t, err)

		err = migrate.Migrate(t.Context(), db, b.dialect, "heal", s, migrate.WithCheckOnly())
		require.ErrorContains(t, err, "failed to record opener in [heal_schema_openers]", "check-only creates nothing")
		require.NoError(t, migrate.Migrate(t.Context(), db, b.dialect, "heal", s, migrate.WithNode("n")))
		requireOpener(t, db, "heal_schema_openers", "heal_s", "n", 1)
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

// requireVersion checks the store's version; readable_by stays 0 because the
// steps of these tests are expand steps.
func requireVersion(t *testing.T, db *sql.DB, table, store string, want int) {
	t.Helper()
	requireRow(t, db, table, store, want, 0)
}

func requireRow(t *testing.T, db *sql.DB, table, store string, version, readableBy int) {
	t.Helper()
	var v, r int
	require.NoError(t, db.QueryRow("SELECT version, readable_by FROM "+table+" WHERE store = $1", store).Scan(&v, &r))
	require.Equal(t, version, v, "version")
	require.Equal(t, readableBy, r, "readable_by")
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

// A store with a pending contract step logs it, and SetProcessOptions applies
// to every Migrate until restored. Not parallel: both replace package-level
// state.
func TestContractStepPendingLogAndProcessOptions(t *testing.T) { //nolint:paralleltest // replaces package-level state
	db := sqliteBackend(t).open(t)
	s := newStore("plog_s", 3)
	s.steps[1].Contract = true
	require.NoError(t, migrate.Migrate(t.Context(), db, migrate.SQLite, "plog", newStore("plog_s", 1)))

	logs := migrate.RecordLogs(t)
	require.NoError(t, migrate.Migrate(t.Context(), db, migrate.SQLite, "plog", s))
	require.Contains(t, logs.Messages(), "store [plog_s]: contract step 2 [create plog_s_t2] is pending and 1 step(s) wait for it; run node migrate --contract once every binary that opens the store knows it")
	requireVersion(t, db, "plog_schema_versions", "plog_s", 1)

	restore := migrate.SetProcessOptions(migrate.WithContract(), migrate.WithForgetOpeners(""))
	t.Cleanup(restore)
	require.NoError(t, migrate.Migrate(t.Context(), db, migrate.SQLite, "plog", s))
	restore()
	requireRow(t, db, "plog_schema_versions", "plog_s", 3, 2)

	// After restore, a contract step waits again.
	o := newStore("plog_o", 2)
	o.steps[1].Contract = true
	require.NoError(t, migrate.Migrate(t.Context(), db, migrate.SQLite, "plog", newStore("plog_o", 1)))
	require.NoError(t, migrate.Migrate(t.Context(), db, migrate.SQLite, "plog", o))
	requireVersion(t, db, "plog_schema_versions", "plog_o", 1)
}

// Migrate reports the store's readable_by through SetReadableBy after every
// successful open, including one that holds a contract step back.
func TestMigrateReportsReadableBy(t *testing.T) {
	t.Parallel()
	db := sqliteBackend(t).open(t)
	ctx := t.Context()

	withContract := newStore("rb_s", 2)
	withContract.steps[1].Contract = true

	// A non-fresh store: step 1 exists already, so the contract step waits.
	require.NoError(t, migrate.Migrate(ctx, db, migrate.SQLite, "rb", newStore("rb_s", 1)))
	nonFresh := &store{name: "rb_s", steps: withContract.steps}
	require.NoError(t, migrate.Migrate(ctx, db, migrate.SQLite, "rb", nonFresh))
	require.Equal(t, int64(0), nonFresh.readableBy.Load())

	// WithContract runs the contract step and reports its version; forget the
	// opener row the earlier non-fresh open left, which would otherwise block it.
	contracted := &store{name: "rb_s", steps: withContract.steps}
	require.NoError(t, migrate.Migrate(ctx, db, migrate.SQLite, "rb", contracted, migrate.WithContract(), migrate.WithForgetOpeners("")))
	require.Equal(t, int64(2), contracted.readableBy.Load())

	// A fresh store runs the contract step at startup and reports its version.
	fresh := &store{name: "rb_fresh_s", steps: []migrate.Step{
		{Version: 1, Name: "create", SQL: []string{"CREATE TABLE rb_fresh_s (id INT)"}},
		{Version: 2, Name: "contract", Contract: true, SQL: []string{"CREATE INDEX rb_fresh_s_id ON rb_fresh_s (id)"}},
	}}
	require.NoError(t, migrate.Migrate(ctx, db, migrate.SQLite, "rb", fresh))
	require.Equal(t, int64(2), fresh.readableBy.Load())

	// WithCheckOnly also reaches recordOpener on success and reports readable_by.
	checked := &store{name: "rb_s", steps: withContract.steps}
	require.NoError(t, migrate.Migrate(ctx, db, migrate.SQLite, "rb", checked, migrate.WithCheckOnly()))
	require.Equal(t, int64(2), checked.readableBy.Load())
}

// TestPostgresMigratorDiesMidStep checks that when the backend of a migrator ends in the
// middle of a step, Postgres rolls the step back and releases its locks: a waiting
// migrator runs an expand step once, and an older binary that waited for a contract
// step opens the store and blocks the contract step for the next migrator.
func TestPostgresMigratorDiesMidStep(t *testing.T) { //nolint:paralleltest // replaces package-level state
	b := postgresBackend(t)
	migrate.SetLockIntervals(t, 10*time.Millisecond, 50*time.Millisecond)
	logs := migrate.RecordLogs(t)

	t.Run("ExpandStep", func(t *testing.T) { //nolint:paralleltest // uses the lock intervals and logger the parent replaced
		dbA, dbB, admin := b.open(t), b.open(t), b.open(t)
		ctx := t.Context()
		d := newDyingStep(t)
		s := &store{name: "dies_s", steps: []migrate.Step{
			{Version: 1, Name: "create", SQL: []string{"CREATE TABLE dies_s (id INT)"}},
			{Version: 2, Name: "insert", Go: func(ctx context.Context, q migrate.Execer) error {
				if _, err := q.ExecContext(ctx, "INSERT INTO dies_s (id) VALUES (1)"); err != nil {
					return err
				}
				return d.block(ctx, q)
			}},
		}}

		doneA := make(chan error, 1)
		go func() { doneA <- migrate.Migrate(ctx, dbA, b.dialect, "dies", s) }()
		recv(t, d.running)
		doneB := make(chan error, 1)
		go func() { doneB <- migrate.Migrate(ctx, dbB, b.dialect, "dies", s, migrate.WithLockTimeout(waitTimeout)) }()
		d.kill(t, admin, logs)

		require.NoError(t, recv(t, doneB))
		require.EqualValues(t, 2, d.calls.Load(), "the waiting migrator ran the step")
		var rows int
		require.NoError(t, admin.QueryRowContext(ctx, "SELECT count(*) FROM dies_s").Scan(&rows))
		require.Equal(t, 1, rows)
		requireVersion(t, admin, "dies_schema_versions", "dies_s", 2)
		d.unblock() // the dead step returns, and its Migrate fails
		require.Error(t, recv(t, doneA))
	})

	// The older opener waits for the dead step's lock on the openers table. Postgres
	// grants it when the dead backend releases its locks, before the waiting migrator
	// polls for the migration lock again, so the opener records its row and blocks
	// the contract step. Once that row is forgotten, a migrator runs the step once.
	t.Run("ContractStep", func(t *testing.T) { //nolint:paralleltest // uses the lock intervals and logger the parent replaced
		dbA, dbB, dbOld, admin := b.open(t), b.open(t), b.open(t), b.open(t)
		ctx := t.Context()
		previous := newStore("diec_s", 1)
		require.NoError(t, migrate.Migrate(ctx, admin, b.dialect, "diec", previous, migrate.WithNode("old")))
		_, err := admin.ExecContext(ctx, "DELETE FROM diec_schema_openers")
		require.NoError(t, err)
		d := newDyingStep(t)
		next := newStore("diec_s", 2)
		next.steps[1].Contract = true
		next.steps[1].Go = d.block

		doneA := make(chan error, 1)
		go func() { doneA <- migrate.Migrate(ctx, dbA, b.dialect, "diec", next, migrate.WithContract()) }()
		recv(t, d.running)
		opened := make(chan error, 1)
		go func() { opened <- migrate.Migrate(ctx, dbOld, b.dialect, "diec", previous, migrate.WithNode("old")) }()
		require.Eventually(t, func() bool {
			var n int
			err := admin.QueryRowContext(ctx, "SELECT count(*) FROM pg_locks WHERE relation = 'diec_schema_openers'::regclass AND NOT granted").Scan(&n)
			return err == nil && n > 0
		}, waitTimeout, 10*time.Millisecond, "the older opener waits for the openers table")
		doneB := make(chan error, 1)
		go func() {
			doneB <- migrate.Migrate(ctx, dbB, b.dialect, "diec", next, migrate.WithContract(), migrate.WithLockTimeout(waitTimeout))
		}()
		d.kill(t, admin, logs)

		require.NoError(t, recv(t, opened))
		require.ErrorIs(t, recv(t, doneB), migrate.ErrOlderOpener)
		require.EqualValues(t, 1, d.calls.Load(), "the contract step ran again")
		requireRow(t, admin, "diec_schema_versions", "diec_s", 1, 0)
		requireNoTable(t, admin, b.dialect, "diec_s_t2")
		requireOpener(t, admin, "diec_schema_openers", "diec_s", "old", 1)

		require.NoError(t, migrate.Migrate(ctx, dbB, b.dialect, "diec", next, migrate.WithContract(), migrate.WithForgetOpeners("old")))
		require.EqualValues(t, 2, d.calls.Load(), "the contract step ran on the next migrator")
		requireRow(t, admin, "diec_schema_versions", "diec_s", 2, 2)
		var tables int
		require.NoError(t, admin.QueryRowContext(ctx, "SELECT count(*) FROM pg_tables WHERE tablename = 'diec_s_t2'").Scan(&tables))
		require.Equal(t, 1, tables)
		d.unblock()
		require.Error(t, recv(t, doneA))
	})
}

// waitTimeout bounds every wait of TestPostgresMigratorDiesMidStep, so a lock that
// is not released fails the test instead of hanging it.
const waitTimeout = 5 * time.Second

// recv returns the next value from c, failing t after waitTimeout.
func recv[T any](t *testing.T, c <-chan T) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(waitTimeout):
		t.Fatalf("no result within %s", waitTimeout)
		var zero T
		return zero
	}
}

// dyingStep is the Go part of a step whose first run blocks until unblock, so the
// test can end that migrator's backend. Later runs return at once.
type dyingStep struct {
	calls            atomic.Int32
	pid              int // backend pid of the first run, set before running closes
	running, release chan struct{}
	once             sync.Once
}

func newDyingStep(t *testing.T) *dyingStep {
	t.Helper()
	d := &dyingStep{running: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(d.unblock)
	return d
}

func (d *dyingStep) block(ctx context.Context, q migrate.Execer) error {
	if d.calls.Add(1) > 1 {
		return nil
	}
	if err := q.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&d.pid); err != nil {
		return err
	}
	close(d.running)
	<-d.release
	return nil
}

func (d *dyingStep) unblock() { d.once.Do(func() { close(d.release) }) }

// kill waits until another migrator logs that it waits for the first run's
// backend, then ends that backend.
func (d *dyingStep) kill(t *testing.T, admin *sql.DB, logs *logging.Recorder) {
	t.Helper()
	require.Eventually(t, func() bool {
		return len(logs.EntriesContaining(fmt.Sprintf("held by pid %d (", d.pid))) > 0
	}, waitTimeout, 10*time.Millisecond)
	var ok bool
	require.NoError(t, admin.QueryRowContext(t.Context(), "SELECT pg_terminate_backend($1)", d.pid).Scan(&ok))
	require.True(t, ok, "backend %d not terminated", d.pid)
}
