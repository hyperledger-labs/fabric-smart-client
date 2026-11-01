# Schema migrations

Every FSC store records its schema version in `<prefix>_schema_versions` (see [Database layout](database-layout.md#schema-versions-table)) and runs its pending schema steps when it is opened. This page is for operators; [DB drivers](db-driver.md#schema-versions) describes the mechanism.

## First start after an upgrade

A node that opens a store with pending steps runs them before the store is used. Replicas that share a Postgres database take an advisory lock per table prefix: one runs the steps, the others wait and log every 10 seconds, then find nothing pending. On SQLite a second process on the same file waits up to `busy_timeout`.

Step 1 of every store creates its tables if they are missing. On the vault and the KVS it also converts an older layout: a Postgres vault `pkey` column of type `TEXT` becomes `BYTEA`, and a primary key of `(pkey, ns)` becomes `(ns, pkey)`. On a large vault this rewrites the table under an exclusive lock and can take a long time.

A vault migrates when its channel opens, after the web listener has started. While it does, `/healthz` answers 200 and `/readyz` answers 503 with reason `migrating`, so a load balancer sends the node no traffic and a liveness probe on `/healthz` does not restart it. The view stores migrate before the listener starts; their step 1 is short unless a large KVS needs its key reordered. For that case, give the first start a startup probe with enough time, or run `node migrate` first.

Back up the database before the first start. A converted `TEXT` vault must never be used by an FSC version that stores `pkey` as `TEXT` (see the warning in [Configuration](../../../configuration.md)), and a binary refuses a store whose `readable_by` is above the highest step it knows, so rolling back across a step means restoring the backup.

## Running migrations separately

Set `skipCreateTable: true` on the persistence and run `node migrate` from a Job or init container with the same `core.yaml` and key material as the node:

```bash
./mynode node migrate --check || ./mynode node migrate
```

With `skipCreateTable: true` the node runs no step. An FSC store with pending steps fails to open with `schema migration pending`, naming the store; this also applies to stores opened after startup, such as a channel added at runtime. `node migrate` reaches the stores the SDKs register at install time, which are every view store, the Fabric endorse tx, metadata and envelope stores, and the vault of every configured channel.

## Database roles on Postgres

Use two roles. The migration role owns the FSC tables, because step 1 runs `CREATE INDEX` and may run `ALTER TABLE` on them, and it creates `<prefix>_schema_versions`. The node role needs DML on the FSC tables and read access to the versions table, and runs with `skipCreateTable: true`:

```sql
CREATE ROLE fsc_migrate LOGIN PASSWORD '...';
CREATE ROLE fsc_node LOGIN PASSWORD '...';
GRANT CREATE ON SCHEMA public TO fsc_migrate;
-- tables created by an earlier release belong to the role that ran it:
REASSIGN OWNED BY old_node_role TO fsc_migrate;
-- tables that later steps create:
ALTER DEFAULT PRIVILEGES FOR ROLE fsc_migrate IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO fsc_node;
ALTER DEFAULT PRIVILEGES FOR ROLE fsc_migrate IN SCHEMA public
  GRANT USAGE ON SEQUENCES TO fsc_node;
-- after `node migrate` ran as fsc_migrate:
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO fsc_node;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO fsc_node;
REVOKE INSERT, UPDATE, DELETE ON fsc_schema_versions FROM fsc_node;
```

Whoever can write the versions table decides which steps a node skips, so keep write access to it with the migration role.

## When a node refuses to start

| Error | Cause | Action |
| --- | --- | --- |
| `store [...] is at version N, binary expects M: schema migration pending; run node migrate` | `skipCreateTable: true` and the store has steps pending | Run `node migrate`, then start the node. |
| `store [...] needs version N, binary knows M: schema is newer than this binary supports` | A newer binary migrated the database | Run the newer binary, or restore a backup taken before its first start. |
| `store [...]: step N [...] failed` | A step failed and rolled back; earlier steps stay applied | Fix the cause in the wrapped error (often a missing privilege) and start again. |
| `table [...] primary key is (...), expected (ns, pkey)` | The table has a layout step 1 does not convert | Fix the table by hand; step 1 left it unchanged. |
| `state table [...] stores pkey as [...], expected [bytea]` | The vault `pkey` column has a type other than `TEXT` or `BYTEA` | Fix the column by hand. |
