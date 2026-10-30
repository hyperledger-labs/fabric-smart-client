/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package migrate records a schema version per SQL store and brings each store
// up to date by running its ordered SQL steps.
//
// # Versions table
//
// Every table prefix has one versions table, <prefix>_schema_versions, with one
// row per store:
//
//	CREATE TABLE IF NOT EXISTS <prefix>_schema_versions (
//	    store       TEXT PRIMARY KEY, -- the store's primary table name
//	    version     INT NOT NULL,     -- highest step applied
//	    readable_by INT NOT NULL      -- lowest step a binary must know to read the store
//	);
//
// The prefix is the configured tablePrefix, lowercased, or "fsc" when empty. A
// missing row means no step has run. readable_by always equals version; the
// column exists so that steps which drop or rewrite data can later record that
// older binaries no longer understand the store, without migrating this table.
//
// # Locking
//
// Migrate reads the store's row without taking the migration lock, so a store
// with nothing pending never waits for a running migration. When steps are
// pending:
//
//   - On Postgres, Migrate takes a dedicated connection and holds a session-level
//     advisory lock keyed by the versions table name while it creates the
//     versions table and runs the steps. Waiting for the lock does not time out
//     (only ctx ends it) and is logged every 10 seconds. Each step runs in its
//     own transaction on that connection.
//   - On SQLite, each step runs in its own BEGIN IMMEDIATE transaction on a
//     dedicated connection from the pool it is given; pass the single-connection
//     write pool. Within a process that pool serializes migrators; across
//     processes the wait is bounded by the connection's busy_timeout.
//
// Each step's transaction re-reads the row and skips the step if another
// migrator applied it meanwhile, then runs the step's statements and updates the
// row. A failed step rolls back, leaves the row at the previous version, and
// stops the migration.
//
// # Rules for steps
//
//   - Steps are numbered 1, 2, 3, ... in the order they run.
//   - A step that has been released is never edited, reordered or removed:
//     databases that already applied it will not run it again.
//   - Every step keeps the store readable by the previous release, so a node
//     that has not been upgraded yet keeps working against a migrated database.
//
// # Trust boundary
//
// The versions table decides which SQL runs against the database: whoever can
// write it can make a node skip steps or refuse to start. Grant write access to
// it only to the roles that run migrations.
package migrate

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"hash/fnv"
	"reflect"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
)

var logger = logging.MustGetLogger()

var (
	// ErrSchemaTooNew is returned when the database was migrated by a binary
	// that knows more steps than this one, so this binary cannot read it.
	ErrSchemaTooNew = errors.New("schema is newer than this binary supports")
	// ErrPending is returned by WithCheckOnly when the store has steps to run.
	ErrPending = errors.New("schema migration pending")
)

// Dialect selects the SQL dialect and the locking strategy.
type Dialect string

const (
	SQLite   Dialect = "sqlite"
	Postgres Dialect = "postgres"
)

// Step is one schema change. SQL runs in a single transaction together with the
// update of the store's version, so a step cannot hold statements that refuse
// to run inside a transaction, such as Postgres CREATE INDEX CONCURRENTLY.
type Step struct {
	// Version is the step's position, starting at 1.
	Version int
	// Name describes the step in logs and errors.
	Name string
	// SQL holds the statements to run, in order.
	SQL []string
}

// Versioned is implemented by stores whose schema is managed by Migrate.
type Versioned interface {
	// StoreName identifies the store's row in the versions table. It is the
	// store's primary table name, which is unique per database.
	StoreName() string
	// Steps returns all steps of the store, ordered by Version.
	Steps() []Step
}

type options struct {
	checkOnly bool
}

// Option configures Migrate.
type Option func(*options)

// WithCheckOnly makes Migrate run no step and create nothing. It returns
// ErrPending if any step is pending.
func WithCheckOnly() Option {
	return func(o *options) { o.checkOnly = true }
}

var (
	// lockPollInterval is how often a waiting Postgres migrator retries the lock.
	lockPollInterval = 500 * time.Millisecond
	// lockLogInterval is how often a waiting Postgres migrator logs that it waits.
	lockLogInterval = 10 * time.Second
)

var tableNames = db.NewTableNameCreator("fsc")

// CreateSchema prepares obj's schema: it runs Migrate if obj implements
// Versioned, and obj.CreateSchema otherwise. db is the pool Migrate uses; see
// the package documentation for which pool each dialect needs.
func CreateSchema(ctx context.Context, db *sql.DB, d Dialect, prefix string, obj drivercommon.DBObject) error {
	if v, ok := obj.(Versioned); ok {
		return Migrate(ctx, db, d, prefix, v)
	}
	return obj.CreateSchema()
}

// Migrate brings v's schema up to date: it runs every step of v whose version
// is above the one recorded in <prefix>_schema_versions, records each applied
// step, and returns once v is at its latest version. It returns ErrSchemaTooNew
// if the database records a readable_by above v's latest step, ErrPending with
// WithCheckOnly when steps are pending, and an error naming the store and the
// step when a step fails. The package documentation describes the locking.
func Migrate(ctx context.Context, db *sql.DB, d Dialect, prefix string, v Versioned, opts ...Option) error {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	store, steps := v.StoreName(), v.Steps()
	for i, s := range steps {
		if s.Version != i+1 {
			return errors.Errorf("store [%s]: step [%s] has version %d, expected %d", store, s.Name, s.Version, i+1)
		}
	}
	table, err := tableNames.CreateTableName(prefix, "schema_versions")
	if err != nil {
		return errors.Wrapf(err, "store [%s]: invalid table prefix", store)
	}
	if err := register(db, table, store, v); err != nil {
		return err
	}

	version, err := readVersion(ctx, db, d, table, store, len(steps))
	if err != nil || version >= len(steps) {
		return err
	}
	if o.checkOnly {
		return errors.Wrapf(ErrPending, "store [%s] is at version %d, binary expects %d", store, version, len(steps))
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return errors.Wrapf(err, "store [%s]: failed to get connection", store)
	}
	defer func() { _ = conn.Close() }()
	if d == Postgres {
		unlock, err := lock(ctx, conn, table, store)
		if err != nil {
			return err
		}
		defer unlock()
	}
	if _, err := conn.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+table+" (store TEXT PRIMARY KEY, version INT NOT NULL, readable_by INT NOT NULL)"); err != nil {
		return errors.Wrapf(err, "store [%s]: failed to create [%s]", store, table)
	}
	for _, s := range steps[version:] {
		if err := apply(ctx, conn, d, table, store, s, len(steps)); err != nil {
			return err
		}
	}
	return nil
}

// registeredKey identifies one version row: the drivers share one pool per
// data source, so the pool stands for the database.
type registeredKey struct {
	db           *sql.DB
	table, store string
}

// registered maps each version row to the type that uses it, so two different
// stores cannot share one version row in a process.
var (
	registeredMu sync.Mutex
	registered   = map[registeredKey]reflect.Type{}
)

func register(db *sql.DB, table, store string, v Versioned) error {
	registeredMu.Lock()
	defer registeredMu.Unlock()
	key, typ := registeredKey{db: db, table: table, store: store}, reflect.TypeOf(v)
	if other, ok := registered[key]; ok && other != typ {
		return errors.Errorf("store [%s] in [%s] is used by both %v and %v", store, table, other, typ)
	}
	registered[key] = typ
	return nil
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// readVersion returns the store's recorded version, 0 if the versions table or
// the row does not exist. It returns ErrSchemaTooNew if readable_by is above
// latest.
func readVersion(ctx context.Context, q querier, d Dialect, table, store string, latest int) (int, error) {
	exists := "SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = $1)"
	if d == Postgres {
		exists = "SELECT to_regclass($1) IS NOT NULL"
	}
	var ok bool
	if err := q.QueryRowContext(ctx, exists, table).Scan(&ok); err != nil {
		return 0, errors.Wrapf(err, "store [%s]: failed to look up [%s]", store, table)
	}
	if !ok {
		return 0, nil
	}
	return readRow(ctx, q, table, store, latest)
}

// readRow is readVersion for a versions table that exists.
func readRow(ctx context.Context, q querier, table, store string, latest int) (int, error) {
	var version, readableBy int
	err := q.QueryRowContext(ctx, "SELECT version, readable_by FROM "+table+" WHERE store = $1", store).Scan(&version, &readableBy) //nolint:gosec // table name comes from configuration, not user input
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, errors.Wrapf(err, "store [%s]: failed to read [%s]", store, table)
	}
	if version < 0 || readableBy < 0 || readableBy > version {
		return 0, errors.Errorf("store [%s]: invalid row in [%s]: version %d, readable_by %d", store, table, version, readableBy)
	}
	if readableBy > latest {
		return 0, errors.Wrapf(ErrSchemaTooNew, "store [%s] needs version %d, binary knows %d", store, readableBy, latest)
	}
	return version, nil
}

// apply runs s in one transaction on conn and records it, unless the row shows
// that another migrator applied s meanwhile. SQLite uses BEGIN IMMEDIATE so the
// transaction holds the write lock from the start and cannot fail on upgrading
// a read lock.
func apply(ctx context.Context, conn *sql.Conn, d Dialect, table, store string, s Step, latest int) (err error) {
	begin := "BEGIN"
	if d == SQLite {
		begin = "BEGIN IMMEDIATE"
	}
	if _, err := conn.ExecContext(ctx, begin); err != nil {
		return errors.Wrapf(err, "store [%s]: failed to begin step %d [%s]", store, s.Version, s.Name)
	}
	defer func() {
		if err == nil {
			_, err = conn.ExecContext(ctx, "COMMIT")
		}
		if err != nil {
			if _, rbErr := conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK"); rbErr != nil {
				discard(conn)
			}
		}
	}()

	version, err := readRow(ctx, conn, table, store, latest)
	if err != nil || version >= s.Version {
		return err
	}
	logger.Infof("store [%s]: running schema step %d [%s]", store, s.Version, s.Name)
	for _, stmt := range s.SQL {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return errors.Wrapf(err, "store [%s]: step %d [%s] failed", store, s.Version, s.Name)
		}
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO "+table+" (store, version, readable_by) VALUES ($1, $2, $2) ON CONFLICT (store) DO UPDATE SET version = excluded.version, readable_by = excluded.readable_by", store, s.Version); err != nil { //nolint:gosec // table name comes from configuration, not user input
		return errors.Wrapf(err, "store [%s]: failed to record step %d [%s]", store, s.Version, s.Name)
	}
	return nil
}

// lock takes the Postgres advisory lock for table on conn, polling until it is
// granted or ctx ends. pg_try_advisory_lock never waits inside the server, so
// lock_timeout and statement_timeout do not apply to the wait.
func lock(ctx context.Context, conn *sql.Conn, table, store string) (unlock func(), err error) {
	key := lockKey(table)
	start, lastLog := time.Now(), time.Now()
	for {
		var ok bool
		if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&ok); err != nil {
			discard(conn) // the server may have granted the lock before the error
			return nil, errors.Wrapf(err, "store [%s]: failed to lock [%s]", store, table)
		}
		if ok {
			break
		}
		if time.Since(lastLog) >= lockLogInterval {
			logger.Infof("store [%s]: waiting %s for another node to finish migrating [%s]", store, time.Since(start).Round(time.Second), table)
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			return nil, errors.Wrapf(ctx.Err(), "store [%s]: waiting for migration lock on [%s]", store, table)
		case <-time.After(lockPollInterval):
		}
	}
	return func() {
		// Unlock even when ctx is done, so the connection can return to the pool.
		if _, err := conn.ExecContext(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", key); err != nil {
			discard(conn)
		}
	}, nil
}

// lockKey derives the advisory lock key from the versions table name, so
// migrators of one prefix exclude each other and other prefixes do not wait.
func lockKey(table string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(table))
	return int64(h.Sum64()) //nolint:gosec // any 64-bit value is a valid lock key
}

// discard makes the pool close conn instead of reusing it, so a connection
// left inside a transaction or holding a lock never serves another caller.
func discard(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
}
