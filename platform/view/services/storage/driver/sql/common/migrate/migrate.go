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
//	    readable_by INT NOT NULL      -- highest contract step applied, 0 if none
//	);
//
// The prefix is the configured tablePrefix, lowercased, or "fsc" when empty. A
// missing row means no step has run. A binary opens a store whose readable_by is
// at most the latest step it knows, also when version is above it: such a binary
// lacks only steps that keep the store readable for it.
//
// # Openers table
//
// <prefix>_schema_openers records which binaries use each store. Migrate
// creates it together with the versions table, and on a store with nothing
// pending if it is missing:
//
//	CREATE TABLE IF NOT EXISTS <prefix>_schema_openers (
//	    store        TEXT NOT NULL,      -- as in the versions table
//	    node         TEXT NOT NULL,      -- the node's FSC id, see WithNode
//	    highest_step INT NOT NULL,       -- the latest step the opening binary knows
//	    opened_at    TIMESTAMP NOT NULL, -- the database's clock (UTC) at the last open
//	    PRIMARY KEY (store, node, highest_step)
//	);
//
// Every successful Migrate, including one WithCheckOnly, upserts the row of its
// store, node and latest step, and then reads readable_by again, so a binary
// that opens the store while a contract step runs refuses it once the step has
// committed. Replicas of one node share an FSC id, so the key includes
// highest_step: replicas on different binaries keep separate rows, and a row
// stays until WithForgetOpeners deletes it.
//
// # Locking
//
// Migrate reads the store's row without taking the migration lock, so a store
// with nothing pending never waits for a running migration. When steps are
// pending:
//
//   - On Postgres, Migrate takes a dedicated connection and holds a session-level
//     advisory lock keyed by the versions table name while it creates the
//     versions and openers tables and runs the steps. Every 10 seconds a waiting
//     migrator logs the session holding the lock: its pid, application_name,
//     state and how long its current statement has run. The wait ends with ctx,
//     or with an error naming the holder after WithLockTimeout. Each step runs in
//     its own transaction on that connection.
//   - On SQLite, each step runs in its own BEGIN IMMEDIATE transaction on a
//     dedicated connection from the pool it is given; pass the single-connection
//     write pool. Within a process that pool serializes migrators; across
//     processes the wait is bounded by the connection's busy_timeout.
//
// InProgress reports whether a Migrate call of this process waits for the lock or
// runs steps; a node reports it on /readyz.
//
// Each step's transaction re-reads the row and skips the step if another
// migrator applied it meanwhile, then runs the step's statements and updates the
// row. A failed step rolls back, leaves the row at the previous version, and
// stops the migration.
//
// # Contract steps
//
// An expand step, the default, leaves the store readable by binaries that lack
// it: it adds tables, columns or indexes. A contract step (Step.Contract) drops
// or rewrites what such binaries read. Its transaction raises readable_by to its
// version, so binaries that lack it refuse the store with ErrSchemaTooNew
// instead of misreading it.
//
// Migrate runs a contract step only WithContract, or on a store that has no
// row yet, which no binary reads. Otherwise it stops before the step, logs that
// the step is pending, and returns with the store at its current version; later
// steps wait too. The binary that ships a contract step therefore works on the
// store both before and after it.
//
// Before a contract step runs, its transaction reads the openers table and
// fails with ErrOlderOpener if a row of the store names a highest_step below the
// step's version. On Postgres it first locks the openers table against writes,
// so an open that records itself meanwhile waits until the step commits; on
// SQLite the step's BEGIN IMMEDIATE does the same.
//
// # Rules for steps
//
//   - Steps are numbered 1, 2, 3, ... in the order they run.
//   - A step that has been released is never edited, reordered or removed:
//     databases that already applied it will not run it again. Step.Hash and
//     the migratetest package check this for the SQL part of a step; a Go
//     function of a released step must not change either.
//   - A step that is not a contract step keeps the store readable by binaries
//     that lack it, so a node that has not been upgraded yet keeps working
//     against a migrated database.
//   - A contract step first rewrites rows still in the old form, which binaries
//     that lack it may have written until it ran, and then drops what they need.
//
// # Trust boundary
//
// The versions table decides which SQL runs against the database: whoever can
// write it can make a node skip steps or refuse to start. Grant write access to
// it only to the roles that run migrations.
package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	// Its text names `node migrate`, the FSC command that runs them.
	ErrPending = errors.New("schema migration pending; run node migrate")
	// ErrOlderOpener is returned when a contract step cannot run because the
	// openers table records a binary that lacks the step.
	ErrOlderOpener = errors.New("store is opened by a binary that lacks the contract step")
)

// errHeldBack stops a migration before a contract step it may not run.
var errHeldBack = errors.New("contract step held back")

// Dialect selects the SQL dialect and the locking strategy.
type Dialect string

const (
	SQLite   Dialect = "sqlite"
	Postgres Dialect = "postgres"
)

// Step is one schema change. SQL and then Go run in a single transaction
// together with the update of the store's version, so a step cannot hold
// statements that refuse to run inside a transaction, such as Postgres CREATE
// INDEX CONCURRENTLY.
type Step struct {
	// Version is the step's position, starting at 1.
	Version int
	// Name describes the step in logs and errors.
	Name string
	// SQL holds the statements to run, in order.
	SQL []string
	// Go, if set, runs after SQL, for changes SQL cannot express such as
	// backfilling a value computed in Go. q runs statements in the step's
	// transaction; Go must not run BEGIN, COMMIT or ROLLBACK. An error rolls
	// the step back.
	Go func(ctx context.Context, q Execer) error
	// Contract marks a step that drops or rewrites what binaries without it
	// read; see the package documentation.
	Contract bool
}

// Hash returns the hex SHA-256 of the step's version, name, SQL statements and,
// for a contract step, the Contract flag. Each field is length-prefixed, so steps
// that differ in any field, or whose statements are split differently, hash
// differently. Go is not covered.
func (s Step) Hash() string {
	h := sha256.New()
	write := func(v string) {
		_ = binary.Write(h, binary.BigEndian, uint64(len(v)))
		_, _ = h.Write([]byte(v))
	}
	write(strconv.Itoa(s.Version))
	write(s.Name)
	_ = binary.Write(h, binary.BigEndian, uint64(len(s.SQL)))
	for _, stmt := range s.SQL {
		write(stmt)
	}
	if s.Contract {
		write("contract") // after the counted statements, so it cannot pass for one
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Execer runs statements. *sql.DB, *sql.Conn and *sql.Tx implement it.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
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
	checkOnly     bool
	contract      bool
	node          string
	forgetOpeners []string
	lockTimeout   time.Duration
}

// Option configures Migrate.
type Option func(*options)

// processOptions apply before the options of every Migrate call; see
// SetProcessOptions.
var (
	processOptionsMu sync.Mutex
	processOptions   []Option
)

// SetProcessOptions makes every Migrate call of this process apply opts before
// its own options, until restore is called. node migrate uses it to pass its
// flags to the stores that the SDKs open while it runs.
func SetProcessOptions(opts ...Option) (restore func()) {
	processOptionsMu.Lock()
	defer processOptionsMu.Unlock()
	old := processOptions
	processOptions = opts
	return func() {
		processOptionsMu.Lock()
		defer processOptionsMu.Unlock()
		processOptions = old
	}
}

func newOptions(opts []Option) options {
	processOptionsMu.Lock()
	opts = append(slices.Clone(processOptions), opts...)
	processOptionsMu.Unlock()
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithCheckOnly makes Migrate run no step and create nothing. It returns
// ErrPending if any step is pending. CreateSchema with WithCheckOnly does not
// call CreateSchema on a store that is not Versioned. This is the behavior a
// store needs under skipCreateTable: true.
func WithCheckOnly() Option {
	return func(o *options) { o.checkOnly = true }
}

// WithContract makes Migrate run pending contract steps; see the package
// documentation. With WithCheckOnly, pending contract steps count as pending.
func WithContract() Option {
	return func(o *options) { o.contract = true }
}

// WithForgetOpeners makes Migrate delete the rows of nodes from the store's
// openers table before it runs any step, so that a contract step no longer
// waits for them. It has no effect WithCheckOnly. Replicas of one node share
// its FSC id: forget a node only once none of its replicas runs a binary that
// lacks the contract step.
func WithForgetOpeners(nodes ...string) Option {
	return func(o *options) { o.forgetOpeners = append(o.forgetOpeners, nodes...) }
}

// WithNode sets the node recorded in the openers table, the node's FSC id.
// Without it the node is recorded as the empty string.
func WithNode(id string) Option {
	return func(o *options) { o.node = id }
}

// WithLockTimeout bounds how long a Postgres migrator waits for the migration
// lock; it then fails with an error that names the holder. Zero, the default,
// waits until ctx ends.
func WithLockTimeout(d time.Duration) Option {
	return func(o *options) { o.lockTimeout = d }
}

var (
	// lockPollInterval is how often a waiting Postgres migrator retries the lock.
	lockPollInterval = 500 * time.Millisecond
	// lockLogInterval is how often a waiting Postgres migrator logs that it waits.
	lockLogInterval = 10 * time.Second
)

var tableNames = db.NewTableNameCreator("fsc")

// inProgress counts the Migrate calls of this process that have steps pending and
// are waiting for the migration lock or running steps.
var inProgress atomic.Int64

// InProgress reports whether a Migrate call of this process is waiting for the
// migration lock or running steps. A node reports it as not ready while it is.
func InProgress() bool { return inProgress.Load() > 0 }

// CreateSchema prepares obj's schema: it runs Migrate with opts if obj
// implements Versioned, and obj.CreateSchema otherwise. db is the pool Migrate
// uses; see the package documentation for which pool each dialect needs.
func CreateSchema(ctx context.Context, db *sql.DB, d Dialect, prefix string, obj drivercommon.DBObject, opts ...Option) error {
	if v, ok := obj.(Versioned); ok {
		return Migrate(ctx, db, d, prefix, v, opts...)
	}
	if newOptions(opts).checkOnly {
		return nil
	}
	return obj.CreateSchema()
}

// Migrate brings v's schema up to date: it runs every step of v whose version
// is above the one recorded in <prefix>_schema_versions, up to the first
// contract step it may not run, records each applied step, and records the
// opener in <prefix>_schema_openers. It returns ErrSchemaTooNew if the database
// records a readable_by above v's latest step, ErrPending with WithCheckOnly
// when steps it would run are pending, naming each pending step, ErrOlderOpener
// when a binary that lacks a contract step opens the store, and an error naming
// the store and the step when a step fails. The package documentation describes
// contract steps and the locking.
func Migrate(ctx context.Context, db *sql.DB, d Dialect, prefix string, v Versioned, opts ...Option) error {
	o := newOptions(opts)
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
	openers, err := tableNames.CreateTableName(prefix, "schema_openers")
	if err != nil {
		return errors.Wrapf(err, "store [%s]: invalid table prefix", store)
	}
	if err := register(db, table, store, v); err != nil {
		return err
	}

	version, err := readVersion(ctx, db, d, table, store, len(steps))
	if err != nil {
		return err
	}
	// A binary behind the store's version, which readable_by allowed, has nothing pending.
	pending := steps[min(version, len(steps)):]
	// A store without a row has no reader that a contract step could break.
	runnable := pending
	if !o.contract && version > 0 {
		if i := slices.IndexFunc(pending, func(s Step) bool { return s.Contract }); i >= 0 {
			runnable = pending[:i]
			logger.Warnf("store [%s]: contract step %d [%s] is pending and %d step(s) wait for it; run node migrate --contract once every binary that opens the store knows it", store, pending[i].Version, pending[i].Name, len(pending)-i-1)
		}
	}
	if o.checkOnly && len(runnable) > 0 {
		return errors.Wrapf(ErrPending, "store [%s] is at version %d, binary expects %d; pending: %s", store, version, len(steps), describe(pending))
	}
	if !o.checkOnly && len(o.forgetOpeners) > 0 {
		if err := forgetOpeners(ctx, db, d, openers, store, o.forgetOpeners); err != nil {
			return err
		}
	}
	if len(runnable) > 0 && !o.checkOnly {
		if err := run(ctx, db, d, table, openers, store, runnable, len(steps), o, version == 0); err != nil {
			return err
		}
	} else if !o.checkOnly {
		// A database migrated before the openers table existed has none yet.
		ok, err := tableExists(ctx, db, d, openers)
		if err != nil {
			return errors.Wrapf(err, "store [%s]: failed to look up [%s]", store, openers)
		}
		if !ok {
			if err := run(ctx, db, d, table, openers, store, nil, len(steps), o, false); err != nil {
				return err
			}
		}
	}
	if err := recordOpener(ctx, db, d, openers, store, o, len(steps)); err != nil {
		return err
	}
	// A contract step that committed after the first read is visible now. The
	// versions table exists: it is created with the openers table.
	_, _, err = readRow(ctx, db, table, store, len(steps))
	return err
}

// forgetOpeners deletes the rows of nodes for store from the openers table, if
// the table exists.
func forgetOpeners(ctx context.Context, db *sql.DB, d Dialect, openers, store string, nodes []string) error {
	ok, err := tableExists(ctx, db, d, openers)
	if err != nil {
		return errors.Wrapf(err, "store [%s]: failed to look up [%s]", store, openers)
	}
	if !ok {
		return nil
	}
	for _, node := range nodes {
		res, err := db.ExecContext(ctx, "DELETE FROM "+openers+" WHERE store = $1 AND node = $2", store, node) //nolint:gosec // table name comes from configuration, not user input
		if err != nil {
			return errors.Wrapf(err, "store [%s]: failed to forget opener [%s]", store, node)
		}
		n, _ := res.RowsAffected()
		logger.Infof("store [%s]: forgot %d opener row(s) of node [%s]", store, n, node)
	}
	return nil
}

// openersColumns is the column list of the openers table.
const openersColumns = "store TEXT NOT NULL, node TEXT NOT NULL, highest_step INT NOT NULL, opened_at TIMESTAMP NOT NULL, PRIMARY KEY (store, node, highest_step)"

// recordOpener upserts the row of store, o.node and latest with the database's
// current UTC time. Migrate makes sure the table exists unless WithCheckOnly is
// set; then a missing table fails the open. The upsert needs SELECT, INSERT and UPDATE.
func recordOpener(ctx context.Context, db *sql.DB, d Dialect, openers, store string, o options, latest int) error {
	now := "CURRENT_TIMESTAMP" // UTC on SQLite
	if d == Postgres {
		now = "(now() AT TIME ZONE 'UTC')"
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO "+openers+" (store, node, highest_step, opened_at) VALUES ($1, $2, $3, "+now+") ON CONFLICT (store, node, highest_step) DO UPDATE SET opened_at = excluded.opened_at", store, o.node, latest); err != nil { //nolint:gosec // table name comes from configuration, not user input
		return errors.Wrapf(err, "store [%s]: failed to record opener in [%s]", store, openers)
	}
	return nil
}

// describe lists steps as "step <n> [<name>]", comma-separated, with
// " (contract)" after a contract step.
func describe(steps []Step) string {
	names := make([]string, len(steps))
	for i, s := range steps {
		names[i] = fmt.Sprintf("step %d [%s]", s.Version, s.Name)
		if s.Contract {
			names[i] += " (contract)"
		}
	}
	return strings.Join(names, ", ")
}

// run takes the migration lock, creates the versions and openers tables, and
// runs pending in order. Postgres CREATE TABLE IF NOT EXISTS is not safe
// against concurrent callers, so both tables are created under the lock. fresh
// reports that Migrate found the store without a row. A contract step runs only
// with o.contract, or on a fresh store whose earlier steps this call applied;
// otherwise run stops before it.
func run(ctx context.Context, db *sql.DB, d Dialect, table, openers, store string, pending []Step, latest int, o options, fresh bool) error {
	inProgress.Add(1)
	defer inProgress.Add(-1)

	conn, err := db.Conn(ctx)
	if err != nil {
		return errors.Wrapf(err, "store [%s]: failed to get connection", store)
	}
	defer func() { _ = conn.Close() }()
	if d == Postgres {
		unlock, err := lock(ctx, conn, table, store, o.lockTimeout)
		if err != nil {
			return err
		}
		defer unlock()
	}
	for _, t := range [][2]string{
		{table, "store TEXT PRIMARY KEY, version INT NOT NULL, readable_by INT NOT NULL"},
		{openers, openersColumns},
	} {
		if _, err := conn.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+t[0]+" ("+t[1]+")"); err != nil {
			return errors.Wrapf(err, "store [%s]: failed to create [%s]", store, t[0])
		}
	}
	// A store Migrate found without a row stays fresh while this migrator
	// applies each step itself; a step another migrator applied means the
	// store has a reader, so a later contract step waits for WithContract.
	for _, s := range pending {
		applied, err := apply(ctx, conn, d, table, openers, store, s, latest, o.contract || fresh)
		if errors.Is(err, errHeldBack) {
			logger.Warnf("store [%s]: another node migrated it meanwhile; contract step %d [%s] waits for node migrate --contract", store, s.Version, s.Name)
			return nil
		}
		if err != nil {
			return err
		}
		fresh = fresh && applied
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

// readVersion returns the store's recorded version, 0 if the versions table or
// the row does not exist. It returns ErrSchemaTooNew if readable_by is above
// latest.
func readVersion(ctx context.Context, q Execer, d Dialect, table, store string, latest int) (int, error) {
	ok, err := tableExists(ctx, q, d, table)
	if err != nil {
		return 0, errors.Wrapf(err, "store [%s]: failed to look up [%s]", store, table)
	}
	if !ok {
		return 0, nil
	}
	version, _, err := readRow(ctx, q, table, store, latest)
	return version, err
}

// tableExists reports whether table exists.
func tableExists(ctx context.Context, q Execer, d Dialect, table string) (bool, error) {
	exists := "SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = $1)"
	if d == Postgres {
		exists = "SELECT to_regclass($1) IS NOT NULL"
	}
	var ok bool
	err := q.QueryRowContext(ctx, exists, table).Scan(&ok)
	return ok, err
}

// readRow is readVersion for a versions table that exists.
func readRow(ctx context.Context, q Execer, table, store string, latest int) (version, readableBy int, err error) {
	err = q.QueryRowContext(ctx, "SELECT version, readable_by FROM "+table+" WHERE store = $1", store).Scan(&version, &readableBy) //nolint:gosec // table name comes from configuration, not user input
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, errors.Wrapf(err, "store [%s]: failed to read [%s]", store, table)
	}
	if version < 0 || readableBy < 0 || readableBy > version {
		return 0, 0, errors.Errorf("store [%s]: invalid row in [%s]: version %d, readable_by %d", store, table, version, readableBy)
	}
	if readableBy > latest {
		return 0, 0, errors.Wrapf(ErrSchemaTooNew, "store [%s] needs version %d, binary knows %d", store, readableBy, latest)
	}
	return version, readableBy, nil
}

// apply runs s in one transaction on conn and records it, unless the row shows
// that another migrator applied s meanwhile. SQLite uses BEGIN IMMEDIATE so the
// transaction holds the write lock from the start and cannot fail on upgrading
// a read lock. A contract step first checks the openers table and raises
// readable_by with the version. Without mayContract a pending contract step
// returns errHeldBack. applied reports whether this call ran s.
func apply(ctx context.Context, conn *sql.Conn, d Dialect, table, openers, store string, s Step, latest int, mayContract bool) (applied bool, err error) {
	begin := "BEGIN"
	if d == SQLite {
		begin = "BEGIN IMMEDIATE"
	}
	if _, err := conn.ExecContext(ctx, begin); err != nil {
		return false, errors.Wrapf(err, "store [%s]: failed to begin step %d [%s]", store, s.Version, s.Name)
	}
	defer func() {
		if err == nil {
			_, err = conn.ExecContext(ctx, "COMMIT")
		}
		if err != nil {
			applied = false
			if _, rbErr := conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK"); rbErr != nil {
				discard(conn)
			}
		}
	}()

	version, readableBy, err := readRow(ctx, conn, table, store, latest)
	if err != nil || version >= s.Version {
		return false, err
	}
	if s.Contract {
		if !mayContract {
			return false, errHeldBack
		}
		if err := checkOpeners(ctx, conn, d, openers, store, s); err != nil {
			return false, err
		}
		readableBy = s.Version
	}
	logger.Infof("store [%s]: running schema step %d [%s]", store, s.Version, s.Name)
	for _, stmt := range s.SQL {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return false, errors.Wrapf(err, "store [%s]: step %d [%s] failed", store, s.Version, s.Name)
		}
	}
	if s.Go != nil {
		if err := s.Go(ctx, conn); err != nil {
			return false, errors.Wrapf(err, "store [%s]: step %d [%s] failed", store, s.Version, s.Name)
		}
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO "+table+" (store, version, readable_by) VALUES ($1, $2, $3) ON CONFLICT (store) DO UPDATE SET version = excluded.version, readable_by = excluded.readable_by", store, s.Version, readableBy); err != nil { //nolint:gosec // table name comes from configuration, not user input
		return false, errors.Wrapf(err, "store [%s]: failed to record step %d [%s]", store, s.Version, s.Name)
	}
	return true, nil
}

// checkOpeners fails with ErrOlderOpener, naming each row, if the openers table
// records a binary that opens store and lacks the contract step s. It runs in
// the step's transaction on q. On Postgres it locks the openers table against
// writes until the transaction ends, so no open records itself between this
// check and the step's commit.
func checkOpeners(ctx context.Context, q Execer, d Dialect, openers, store string, s Step) error {
	if d == Postgres {
		if _, err := q.ExecContext(ctx, "LOCK TABLE "+openers+" IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			return errors.Wrapf(err, "store [%s]: failed to lock [%s] for contract step %d [%s]", store, openers, s.Version, s.Name)
		}
	}
	rows, err := q.QueryContext(ctx, "SELECT node, highest_step, opened_at FROM "+openers+" WHERE store = $1 AND highest_step < $2 ORDER BY node, highest_step", store, s.Version) //nolint:gosec // table name comes from configuration, not user input
	if err != nil {
		return errors.Wrapf(err, "store [%s]: failed to read [%s]", store, openers)
	}
	defer func() { _ = rows.Close() }()
	var older []string
	for rows.Next() {
		var (
			node    string
			highest int
			at      time.Time
		)
		if err := rows.Scan(&node, &highest, &at); err != nil {
			return errors.Wrapf(err, "store [%s]: failed to read [%s]", store, openers)
		}
		older = append(older, fmt.Sprintf("node [%s] knows step %d, last opened %s UTC", node, highest, at.Format(time.DateTime)))
	}
	if err := rows.Err(); err != nil {
		return errors.Wrapf(err, "store [%s]: failed to read [%s]", store, openers)
	}
	if len(older) > 0 {
		return errors.Wrapf(ErrOlderOpener, "store [%s]: contract step %d [%s] not run: %s; upgrade them, or run node migrate --forget-opener <node> for a node none of whose replicas runs that binary anymore", store, s.Version, s.Name, strings.Join(older, "; "))
	}
	return nil
}

// lock takes the Postgres advisory lock for table on conn, polling until it is
// granted, ctx ends, or timeout (if positive) elapses. pg_try_advisory_lock
// never waits inside the server, so lock_timeout and statement_timeout do not
// apply to the wait.
func lock(ctx context.Context, conn *sql.Conn, table, store string, timeout time.Duration) (unlock func(), err error) {
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
		if timeout > 0 && time.Since(start) >= timeout {
			return nil, errors.Errorf("store [%s]: migration lock on [%s] not granted within %s; held by %s", store, table, timeout, lockHolder(ctx, conn, key))
		}
		if time.Since(lastLog) >= lockLogInterval {
			logger.Infof("store [%s]: waiting %s for the migration lock on [%s], held by %s", store, time.Since(start).Round(time.Second), table, lockHolder(ctx, conn, key))
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

// lockHolder describes the session holding the advisory lock key in the current
// database, for logs and errors. A bigint advisory key is split into classid
// (high 32 bits) and objid (low 32 bits) with objsubid 1. pg_stat_activity hides
// the state and statement start of another role's session unless the reader has
// pg_read_all_stats; they are then reported as unknown.
func lockHolder(ctx context.Context, conn *sql.Conn, key int64) string {
	var (
		pid                int
		app, state, runFor sql.NullString
	)
	err := conn.QueryRowContext(ctx, `SELECT a.pid, a.application_name, a.state, (clock_timestamp() - a.query_start)::text
		FROM pg_locks l JOIN pg_stat_activity a ON a.pid = l.pid
		WHERE l.locktype = 'advisory' AND l.granted AND l.objsubid = 1
		  AND l.database = (SELECT oid FROM pg_database WHERE datname = current_database())
		  AND l.classid::bigint = $1 AND l.objid::bigint = $2`,
		int64(uint64(key)>>32), int64(uint32(key))).Scan(&pid, &app, &state, &runFor) //nolint:gosec // splitting the key into its two unsigned halves
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "no session (just released)"
		}
		return fmt.Sprintf("an unknown session (%v)", err)
	}
	unknown := func(v sql.NullString) string {
		if v.Valid {
			return v.String
		}
		return "unknown"
	}
	return fmt.Sprintf("pid %d (application_name [%s], state %s, statement running for %s)", pid, app.String, unknown(state), unknown(runFor))
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
