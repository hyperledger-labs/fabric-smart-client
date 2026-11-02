# Schema migrations

Every FSC store records its schema version in `<prefix>_schema_versions` (see [Database layout](database-layout.md#schema-versions-table)) and runs its pending schema steps when it is opened. This page is for operators; [DB drivers](db-driver.md#schema-versions) describes the mechanism.

## First start after an upgrade

A node that opens a store with pending steps runs them before the store is used. Replicas that share a Postgres database take an advisory lock per table prefix: one runs the steps, the others wait, then find nothing pending. Every 10 seconds a waiting node logs the session that holds the lock:

```text
store [fsc_kvs]: waiting 40s for the migration lock on [fsc_schema_versions], held by pid 4711 (application_name [mynode-0], state active, statement running for 00:00:38.2)
```

The node waits without limit, so a replica behind a long step keeps waiting. Set `fsc.storage.migrationLockTimeout` (see [Configuration](../../../configuration.md)) to make it fail instead, with an error that names the holder. Set `application_name` in each node's `dataSource` to its pod name so the log names the holding pod. The state and statement time of a session of another role show as `unknown` unless the waiting role has `pg_read_all_stats`. On SQLite a second process on the same file waits up to `busy_timeout`.

Step 1 of every store creates its tables if they are missing. On the vault and the KVS it also converts an older layout: a Postgres vault `pkey` column of type `TEXT` becomes `BYTEA`, and a primary key of `(pkey, ns)` becomes `(ns, pkey)`. On a large vault this rewrites the table under an exclusive lock and can take a long time.

A vault migrates when its channel opens, after the web listener has started. While it does, `/healthz` answers 200 and `/readyz` answers 503 with reason `migrating`, so a load balancer sends the node no traffic and a liveness probe on `/healthz` does not restart it. The view stores migrate before the listener starts; their step 1 is short unless a large KVS needs its key reordered. For that case, give the first start a startup probe with enough time, or run `node migrate` first.

Back up the database before the first start. A converted `TEXT` vault must never be used by an FSC version that stores `pkey` as `TEXT` (see the warning in [Configuration](../../../configuration.md)), and a binary refuses a store whose `readable_by` is above the highest step it knows, so rolling back across a step means restoring the backup.

## Running migrations separately

Set `skipCreateTable: true` on the persistence and run `node migrate` from a Job or init container with the same `core.yaml` and key material as the node:

```bash
./mynode node migrate --check || ./mynode node migrate
```

`node migrate` exits 0 when every store is current (or, without `--check`, has been migrated), 10 when `--check` finds pending steps, 11 when a store was migrated by a newer binary, 1 when it could not check or migrate a store, and 2 on invalid arguments. `--check` opens every store and lists each pending one with its steps, for example `store [fsc_kvs] is at version 0, binary expects 1; pending: step 1 [create table]`. A store that names no persistence while `fsc.persistences.default` is not defined fails the command: it would use memory, and nothing would be migrated.

With `skipCreateTable: true` the node runs no step. An FSC store with pending steps fails to open with `schema migration pending`, naming the store; this also applies to stores opened after startup, such as a channel added at runtime. `node migrate` reaches the stores the SDKs register at install time, which are every view store, the Fabric endorse tx, metadata and envelope stores, and the vault of every configured channel.

## Database roles on Postgres

Use two roles. The migration role owns the FSC tables, because step 1 runs `CREATE INDEX` and may run `ALTER TABLE` on them, and it creates `<prefix>_schema_versions` and `<prefix>_schema_openers`. The node role needs DML on the FSC tables, read access to the versions table, and `SELECT`, `INSERT` and `UPDATE` on the openers table, where every open records the node with an upsert; it runs with `skipCreateTable: true`:

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
REVOKE DELETE ON fsc_schema_openers FROM fsc_node;
```

Whoever can write the versions table decides which steps a node skips, so keep write access to it with the migration role.

## When a node refuses to start

| Error | Cause | Action |
| --- | --- | --- |
| `store [...] is at version N, binary expects M; pending: step ... : schema migration pending; run node migrate` | `skipCreateTable: true` and the store has steps pending | Run `node migrate`, then start the node. |
| `store [...]: migration lock on [...] not granted within ...; held by pid N (...)` | `fsc.storage.migrationLockTimeout` elapsed while another session held the lock | See [A node waits for the migration lock](#a-node-waits-for-the-migration-lock). |
| `store [...]: failed to record opener in [<prefix>_schema_openers]: ... permission denied` | The node role cannot write the openers table | Grant `SELECT, INSERT, UPDATE` on it; see [Database roles on Postgres](#database-roles-on-postgres). |
| `store [...] needs version N, binary knows M: schema is newer than this binary supports` | A newer binary migrated the database | Run the newer binary, or restore a backup taken before its first start. |
| `store [...]: step N [...] failed` | A step failed and rolled back; earlier steps stay applied | Fix the cause in the wrapped error (often a missing privilege) and start again. |
| `table [...] primary key is (...), expected (ns, pkey)` | The table has a layout step 1 does not convert | Fix the table by hand; step 1 left it unchanged. |
| `state table [...] stores pkey as [...], expected [bytea]` | The vault `pkey` column has a type other than `TEXT` or `BYTEA` | Fix the column by hand. |

## A node waits for the migration lock

A waiting node logs the holder's `pid` and `application_name` every 10 seconds. A holder in state `active` whose statement keeps running is a step at work, such as a vault rewrite; let it finish. A holder in state `idle` holds the lock without migrating, for example a process that hangs after a step. Look at it, then end its session; the lock is released with the session, and the step it was running rolls back:

```sql
SELECT pid, application_name, state, now() - query_start AS running_for, query
FROM pg_stat_activity WHERE pid = 4711;
SELECT pg_terminate_backend(4711);
```

## Which binaries use a store

Every open of a store, including one with `skipCreateTable: true`, records the node's `fsc.id` and the highest step its binary knows in `<prefix>_schema_openers` (see [Database layout](database-layout.md#schema-openers-table)). Replicas of one node share an `fsc.id`; replicas on different binaries keep a row each. Nothing reads the table yet; it is there so that a later step that the previous release cannot read can check that no such binary still opens the store.
