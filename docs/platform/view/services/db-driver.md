# DB Drivers

This document explains FSC persistence, store, and driver concepts.
If you are looking for the physical tables and columns created by those stores, see [Database layout](database-layout.md).
If you are looking for runtime usage and operator inspection guidance, see [Runtime DB access](runtime-db-access.md).

## Project terminology

### Database
A project can support multiple databases, as long as the relations of the DB don't have to interact. The common practice would be to use just one DB per project, e.g. Postgres. We can also have multiple Postgres databases in one project (also not recommended for simplicity purposes). 

### Persistence
A persistence is an instantiation strategy, and it is backed by a specific database.
Multiple persistences can share the same underlying database, but still be different (e.g. different table prefix).

* Each persistence has a `type` that defines the underlying database type.
Depending on the `type`, a persistence (except for `memory`) must define a different set of options (`opts`).
We support the following types:
  * `memory` (backed by an Sqlite database)
  * `postgres` (backed by a Postgres database)
  * `sqlite` (backed by an Sqlite database)

* The `dataSource` defines the concrete underlying DB.
Two persistences that share the same `type` but not `dataSource`, have different underlying databases (although both of these database are of the same type).

* Further properties such as `tablePrefix` and `maxIdleConns` complete the persistence.
Two persistences can share the same `type` and `dataSource` and differ in the rest of the options.
Connection pools are cached by `dataSource` (on Postgres, by `dataSource` and `tls` block), so such persistences share one pool, and the pool settings (`maxOpenConns`, `maxIdleConns`, `maxIdleTime`, and `skipPragmas` on SQLite or `startupConnectTimeout` on Postgres) of the first store opened on it apply to all of them.
`tablePrefix`, `skipCreateTable` and `queryTimeout` stay per persistence.

We can have one or more persistences in a project, as well as one optional fallback persistence (`default`).

### Store
A store is a logical partition of the project storage, e.g. identity info, transactions, ledger state.
It consists of one or more underlying tables and exposes querying functionality through an interface.
A store can also have references to other stores (e.g. foreign keys, joins).
It has one implementation per persistence type, e.g.:
* `memory`: Sqlite DB in memory; the `memory` driver uses the SQLite stores (`sqlite.NewBindingStore` and so on) on an in-memory database
* `postgres.BindingStore`: Postgres DB via remote connection to standalone running DB (e.g. container)
* `sqlite.BindingStore`: Sqlite DB in the local filesystem

However, they all share one common interface such as the following:

```go
type BindingStore interface {
	GetLongTerm(ctx context.Context, ephemeral view.Identity) (view.Identity, error)
	HaveSameBinding(ctx context.Context, this, that view.Identity) (bool, error)
	PutBindings(ctx context.Context, longTerm view.Identity, ephemeral ...view.Identity) error
	ReplaceBinding(ctx context.Context, longTerm, ephemeral view.Identity) error
}

type AuditInfoStore interface {
	GetAuditInfo(ctx context.Context, id view.Identity) ([]byte, error)
	PutAuditInfo(ctx context.Context, id view.Identity, info []byte) error
}
```

The store interfaces are defined in `platform/common/driver/kvs.go`.

### Driver
A driver is responsible for the instantiation of multiple (or usually all) stores.
It has multiple implementations: one for each persistence type.
* `memory.Driver`: Sqlite DB in memory
* `postgres.Driver`: Postgres DB via remote connection to standalone running DB (e.g. container)
* `sqlite.Driver`: Sqlite DB in the local filesystem

Aside to the database-related implementations of the `Driver` interface (i.e. `memory.Driver`, `postgres.Driver`, `sqlite.Driver`), there is a `multiplexed.Driver` that reads the underlying configuration and instantiates the store using the correct driver.

During instantiation, the driver has access to the persistence `opts`.
This way, it takes care of:
* opening the connection pool to the DB (unless a pool for the same `dataSource` is already open)
* preparing the schema (see [Schema versions](#schema-versions)); with `skipCreateTable: true` no schema change runs, and a store with schema steps pending fails to open
* the instantiation of the service that builds and executes the queries to the database

The driver interface, from `platform/view/services/storage/driver/driver.go`:

```go
type Driver interface {
	// NewKVS returns a new KeyValueStore for the passed data source and config
	NewKVS(PersistenceName, ...string) (KeyValueStore, error)
	// NewBinding returns a new BindingStore for the passed data source and config
	NewBinding(PersistenceName, ...string) (BindingStore, error)
	// NewSignerInfo returns a new SignerInfoStore for the passed data source and config
	NewSignerInfo(PersistenceName, ...string) (SignerInfoStore, error)
	// NewAuditInfo returns a new AuditInfoStore for the passed data source and config
	NewAuditInfo(PersistenceName, ...string) (AuditInfoStore, error)
}
```

**Note:** Although these methods have different names and return different stores, their signatures have the same input parameters for the sake of uniformity, code re-use, and maintainability.


## Config

All persistences are defined under `fsc.persistences` in the `core.yaml`. Each persistence has a unique persistence name and there can be one `default` persistence. A name contains only letters, digits, `_` and `-`; a store that references any other name fails to start, and the error does not print the value, which keeps a dataSource placed there by mistake out of the logs.

```yaml
fsc.persistences:
  default:                      # The default persistence if a store doesn't choose explicitly one
    type: postgres
    opts:
      dataSource: dataSource1   # See core.yaml documentation for more details
      maxIdleConns: 3
  my_custom_postgres:           # The persistence name. Defines a new postgres-based persistence
    type: postgres
    opts:
      dataSource: dataSource2   # Will open a new database
      maxIdleConns: 4
      tablePrefix: custom
  my_custom_postgres_2:
    type: postgres
    opts:
      dataSource: dataSource1   # Shares the connection pool of default
      tablePrefix: non_default
  my_custom_mem:
    type: memory                # No options required for memory
  my_custom_sqlite:
    type: sqlite
    opts:
      dataSource: /path/to/sqlite
      tablePrefix: some_prefix
```

Each store has a dedicated configuration in the `core.yaml` file.
Its instantiation is defined in the field `persistence` and it references a persistence name that has already been defined under `fsc.persistences`.
If none is defined, the `default` will be taken, provided there is one.
If there is no `default`, the FSC stores fall back to `memory` and log a warning, so their data is lost on restart.
Creating a store fails if it references a name that is not defined under `fsc.persistences`, or if the referenced persistence sets no `type`.

```yaml
fsc:
  metadata:
    persistence: my_custom_postgres
  auditinfo:
    persistence:  # The default will be chosen
  signerinfo:
    persistence: my_custom_postgres # Will re-use the same persistence
```

The drivers resolve a persistence name to its `fsc.persistences.<name>` block through `platform/view/services/storage/driver/common/config.go`.

`CORE_FSC_PERSISTENCES_<NAME>_OPTS_<KEY>` overrides a persistence option. Write each `_` of the name as `__`: `CORE_FSC_PERSISTENCES_MY__CUSTOM__POSTGRES_OPTS_DATASOURCE` sets the `dataSource` of `my_custom_postgres`. See [Overriding configuration keys](../../../configuration.md#overriding-configuration-keys).

### Transaction isolation levels

Postgres accepts every `driver.IsolationLevel` except `LevelWriteCommitted` and `LevelLinearizable`, which the pgx driver does not support; FSC rejects both before it begins the transaction. `LevelSnapshot` runs as `REPEATABLE READ`.
SQLite accepts only `LevelDefault` and `LevelSerializable` and rejects any other level.

## Schema versions

After building a store, every driver constructor (sqlite, postgres and memory, for both the view and the fabric stores) passes it to `migrate.CreateSchema` in `platform/view/services/storage/driver/sql/common/migrate`:

* a store that implements `migrate.Versioned` is brought up to date by `migrate.Migrate`, which runs its pending steps and records the store's version in `<prefix>_schema_versions` (see [Database layout](database-layout.md#schema-versions-table));
* any other store gets `CreateSchema()`.

Every FSC store (KVS, binding, signer info, audit info, endorse tx, metadata, envelope and vault) implements `migrate.Versioned`. Its `StoreName` is its primary table, for the vault the state table. Step 1 creates the store's tables with `IF NOT EXISTS`, so it accepts an empty database and one created by an earlier release. On the KVS and the vault, step 1 also runs `ReorderPrimaryKeyTx`, and on a Postgres vault `convertTextKeysTx` before it, inside the step's transaction; both change nothing on a table that already has the current layout. `CreateSchema()` on an FSC store still creates the tables and runs the same probes, without recording a version.

The constructors pass the options from `sqlcommon.MigrateOptions`: `migrate.WithNode` with the node's `fsc.id`, which every open records in `<prefix>_schema_openers` (see [Database layout](database-layout.md#schema-openers-table)), and `migrate.WithLockTimeout` with `fsc.storage.migrationLockTimeout`. With `skipCreateTable: true` they also pass `migrate.WithCheckOnly()`: no step runs, no table is created, `CreateSchema()` is not called, and a `Versioned` store with steps pending fails with `migrate.ErrPending`, naming the store, both versions and each pending step. The open still records the opener, so the node role needs `SELECT`, `INSERT` and `UPDATE` on the openers table. A database managed outside the node must therefore be migrated before the node opens it. Run `node migrate` (see [Node CLI](../../../reference/node-cli.md#node-migrate)) with a role that may change the schema, then start the node.

`Migrate` reads the version without taking the migration lock, so a store that is up to date never waits for a running migration. When steps are pending, Postgres migrators of one table prefix exclude each other with an advisory lock; every 10 seconds a waiting node logs the session that holds it, read from `pg_locks` and `pg_stat_activity`, and it waits until `WithLockTimeout` elapses, without limit by default. The lock is held by one session across several transactions, so the migrating node needs a direct connection or a session-pooling proxy, not PgBouncer in transaction-pooling mode. SQLite runs each step in a `BEGIN IMMEDIATE` transaction on the write connection, so a second process on the same file waits at most for the connection's `busy_timeout`; after that its step fails with `SQLITE_BUSY` and the store does not open. A failed step rolls back and the store does not open. A database whose `readable_by` is above the highest step the binary knows fails with `migrate.ErrSchemaTooNew`.

A store whose write format depends on one of its own contract steps implements `migrate.ReadableByObserver`; `Migrate` calls its `SetReadableBy` with the store's `readable_by` after every successful open, including `WithCheckOnly`, so the store learns once the step has run without reading the versions table itself.

### Versioning an application store

A store adopts versioning by adding two methods next to `CreateSchema`. `StoreName` returns the store's primary table name, and `Steps` returns every step in order, starting at version 1:

```go
func (s *OrderStore) StoreName() string { return s.table }

func (s *OrderStore) Steps() []migrate.Step {
	return []migrate.Step{
		{Version: 1, Name: "create orders", SQL: []string{
			"CREATE TABLE IF NOT EXISTS " + s.table + " (id TEXT PRIMARY KEY, amount BIGINT NOT NULL)",
		}},
		{Version: 2, Name: "add orders.note", SQL: []string{
			"ALTER TABLE " + s.table + " ADD COLUMN note TEXT NOT NULL DEFAULT ''",
		}},
	}
}
```

Write step 1 with `IF NOT EXISTS` so that it accepts a database created by the store's earlier `CreateSchema`. Never edit, reorder or remove a released step: a database that applied it does not run it again. A step must leave the store readable by binaries that lack it, unless it is a contract step.

#### Contract steps

Dropping a column, changing a type or moving data into a new form breaks binaries that still read the old form. Split such a change in two:

1. An expand step adds the new form, and the store code of the same release writes both forms and reads the new one, falling back to the old one. Binaries of the previous release keep working.
2. A contract step, in a later release, copies rows that are still only in the old form and then drops it. Set `Contract: true`:

```go
{Version: 3, Name: "drop orders.amount_text", Contract: true, SQL: []string{
	"UPDATE " + s.table + " SET amount = CAST(amount_text AS BIGINT) WHERE amount IS NULL",
	"ALTER TABLE " + s.table + " DROP COLUMN amount_text",
}},
```

Backfill before dropping: until the operator runs the step, replicas of the previous release may have kept writing the old form. On a store that existed before, a node does not run a contract step at startup, only `node migrate --contract` does (see [Contract steps](schema-migrations.md#contract-steps)), so the release that ships the step serves the store both before and after it and must not depend on the step having run. A new store runs every step at startup. `StoreName` must be the store's primary table: a store without a version row whose table exists counts as existing. The step runs in one transaction, which on a large table holds its locks for the whole rewrite. Do not mark a step as a contract step after its release: `Step.Hash` covers the flag.

`Step.Hash` is a SHA-256 over a step's version, name, SQL and `Contract` flag. The `migrate/migratetest` package checks a store's steps against a golden file with one line per step, `<version> <hash> <name>`, and fails when a recorded line changes or disappears. New steps are appended by running the test with `FSC_UPDATE_STEPS=1`, which never rewrites a recorded line. Build the store with fixed table names, since steps embed them:

```go
func TestOrderStoreSteps(t *testing.T) {
	s, err := NewOrderStore(&drivercommon.RWDB{}, "orders")
	require.NoError(t, err)
	migratetest.CheckSteps(t, "testdata/steps/orders.steps", s.Steps())
}
```

The FSC stores keep their golden files under `platform/view/services/storage/driver/sql/testdata/steps` and `platform/fabric/services/db/driver/sql/testdata/steps`, one per dialect and store. On every pull request, `scripts/check-step-files.sh` checks that each step file of the latest release tag is a line prefix of the file at HEAD, so a step cannot be changed in the golden file and the code together. The hash does not cover a step's `Go` function.

A store whose databases may have received manual changes before it adopted versioning needs a step 1 that converges from each of them. Probe the catalog before each change instead of assuming a starting layout:

```go
{Version: 1, Name: "create orders", Go: func(ctx context.Context, q migrate.Execer) error {
	if _, err := q.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+s.table+" (id TEXT PRIMARY KEY, amount BIGINT NOT NULL)"); err != nil {
		return err
	}
	// Postgres: SELECT EXISTS (SELECT 1 FROM information_schema.columns
	//           WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2)
	var has bool
	if err := q.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pragma_table_info($1) WHERE name = $2)", s.table, "note").Scan(&has); err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err := q.ExecContext(ctx, "ALTER TABLE "+s.table+" ADD COLUMN note TEXT NOT NULL DEFAULT ''")
	return err
}},
```

Such a step does its work in `Go`, so its hash covers only its version and name.

A step whose change SQL cannot express sets `Go`, which runs after the step's `SQL` in the same transaction. Typical cases are a backfill computed in Go and a SQLite table rebuild. The function runs its statements through the `migrate.Execer` it receives and must not begin, commit or roll back a transaction. If it returns an error, the step rolls back, `SQL` included:

```go
{Version: 3, Name: "backfill orders.hash", SQL: []string{
	"ALTER TABLE " + s.table + " ADD COLUMN hash TEXT",
}, Go: func(ctx context.Context, q migrate.Execer) error {
	// read rows with q.QueryContext, close them, then write with q.ExecContext
	return nil
}},
```

The rules for released steps cover the Go function too: a database that applied the step never runs a changed function body. A step that rewrites existing data needs a test that checks the rewritten values; the fixture tests under `platform/fabric/services/db/driver/sql/testdata` only check that the rows they wrote still read back after every step.

A store built through the FSC constructors is migrated automatically. A store an application builds with its own constructor calls `migrate.CreateSchema(ctx, writeDB, dialect, tablePrefix, store, opts...)` where it calls `CreateSchema()` today, with `opts` from the persistence's `ConfigProvider.MigrateOptions(skipCreateTable)`, as the FSC constructors do. On SQLite, pass the single-connection write pool. To honor the persistence's `queryTimeout`, derive each statement's context with `sqlcommon.QueryTimeout(cfg.QueryTimeout).Context(ctx)`, and close an iterator of rows with `sqlcommon.NewWrappedIteratorWithCancel`, so that the deadline lasts until the rows are read.

A table that several stores use is versioned by one of them, its owner: only the owner lists steps for the table, so the table has a single row in the versions table. Each store migrates when it is opened, so steps that depend on another store's tables, for example through a foreign key, run after that store has been opened.

### Reaching stores from `node migrate`

`node migrate` installs every SDK without starting it and calls the `node.StoreOpener` that the view SDK registers. That opener calls every entry of the dig value group `fsc-store-openers`. The view SDK provides an entry for its stores and the Fabric SDK one for the endorse tx, metadata and envelope stores and the vault of every channel configured for a network on the generic driver. An application SDK whose stores should be migrated by the command provides an entry during `Install`:

```go
p.Container().Provide(func() viewsdk.StoreOpenerEntry {
	return viewsdk.StoreOpenerEntry{Opener: viewsdk.StoreOpenerFunc(func(ctx context.Context) error {
		return p.Container().Invoke(func(*OrderStore) {})
	})}
})
```

The command calls every entry even if one fails and joins their errors, so an entry should report every store it fails to open rather than stop at the first. An entry whose stores use a `multiplexed.Driver` returns `viewsdk.ErrMemoryFallback` when `FellBackToMemory` reports that one of them uses memory by fallback. SDKs nested in each other share one container, so an entry provided by an inner or outer SDK is reached. Stores that are opened only after `Start`, such as the vault of a channel added at runtime, or that an application opens outside its entry, are not migrated by the command; they migrate when the node opens them, or refuse to open under `skipCreateTable: true`.

### Reporting store schemas

`node version --json` lists the schema steps of the stores in `migrate.Schemas()`. An SDK registers its stores when it is built, before any SDK installs, so the command needs no configuration: the view SDK registers `sql.Schemas` of `platform/view/services/storage/driver/sql`, the Fabric SDK that of `platform/fabric/services/db/driver/sql`. Each list builds its stores on an empty `RWDB` with the default table names, and `TestSteps` checks it against the golden files under `testdata/steps`, so a store with a golden file cannot be missing from the list. An application SDK whose stores should be reported registers them in its constructor:

```go
func NewSDK(registry services.Registry) *SDK {
	migrate.RegisterSchemas("orders", func() []migrate.Schema {
		return []migrate.Schema{
			migrate.NewSchema("orders", migrate.Postgres, func() (migrate.Versioned, error) {
				return NewOrderStore(&drivercommon.RWDB{}, defaultTables)
			}),
		}
	})
	...
}
```

Registering a name again replaces its list, so an SDK built twice is reported once.

## Extending the Driver on the same Project

If you want to add a new store (e.g. `TransactionStore`), you will need to:
* extend the `Driver` interface with one more method that has the same inputs
```go
type Driver interface {
	NewKVS(PersistenceName, ...string) (KeyValueStore, error)
	...
	NewTransaction(PersistenceName, ...string) (TransactionStore, error)
}
```
* implement it for all 4 drivers (DB specific and multiplexed)

## Adding Drivers or Creating Drivers on new Projects

Although in the scope of this project, it is sufficient to extend the existing Driver with the addition of more methods for new stores, it will be necessary to add stores in extending projects.

In that case, you will need to create a new Driver interface that follows the same principles as before (containing only the new stores) and make sure that:
* the `Driver` is implemented by all necessary DBs. If you do not support `memory` in your `core.yaml`, then there is no need to create an extending `Driver`. Similarly, if you want to support more DBs, you will need to extend them accordingly.
* there is a new `multiplexed.Driver` that combines the rest of the `Driver` implementations
* the `Driver` implementations are wired in the SDK
* there is a section in the `core.yaml` that follows the same format and defines the characteristics of the store
