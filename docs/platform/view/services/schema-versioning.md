# Schema versioning

This page is for developers who write a store. It describes how a store declares its schema steps
so that FSC migrates it. [Database drivers](db-driver.md#schema-versions) describes how a driver runs
the steps when it opens a store, and [Schema migrations](schema-migrations.md) how an operator
runs them.

## Versioning an application store

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

## Contract steps

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

A store built through the FSC constructors is migrated automatically. A store an application builds with its own constructor calls `migrate.CreateSchema(ctx, writeDB, dialect, tablePrefix, store, opts...)` in place of `CreateSchema()`, with `opts` from the persistence's `ConfigProvider.MigrateOptions(skipCreateTable)`, as the FSC constructors do. On SQLite, pass the single-connection write pool. To honor the persistence's `queryTimeout`, derive each statement's context with `sqlcommon.QueryTimeout(cfg.QueryTimeout).Context(ctx)`, and close an iterator of rows with `sqlcommon.NewWrappedIteratorWithCancel`, so that the deadline lasts until the rows are read.

A table that several stores use is versioned by one of them, its owner: only the owner lists steps for the table, so the table has a single row in the versions table. Each store migrates when it is opened, so steps that depend on another store's tables, for example through a foreign key, run after that store has been opened.

To have `node migrate` reach the store and `node version --json` report its steps, see
[Reaching stores from `node migrate`](db-driver.md#reaching-stores-from-node-migrate) and
[Reporting store schemas](db-driver.md#reporting-store-schemas).
