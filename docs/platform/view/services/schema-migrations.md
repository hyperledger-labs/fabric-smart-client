# Schema migrations

Every FSC store records its schema version in `<prefix>_schema_versions` (see
[Database layout](database-layout.md#schema-versions-table)) and runs its
pending schema steps when it is opened. This page is for operators;
[Database drivers](db-driver.md#schema-versions) describes the mechanism, and
[Rolling upgrades](../../../upgrade/rolling-upgrade.md) the order of an upgrade
across nodes.

## First start after an upgrade

A node that opens a store with pending steps runs them before the store is used.
Replicas that share a Postgres database take an advisory lock per table prefix:
one runs the steps, the others wait, then find nothing pending. Every 10 seconds
a waiting node logs the session that holds the lock:

```text
store [fsc_kvs]: waiting 40s for the migration lock on [fsc_schema_versions], held by pid 4711 (application_name [mynode-0], state active, statement running for 00:00:38.2)
```

The node waits without limit, so a replica behind a long step keeps waiting. Set
`fsc.storage.migrationLockTimeout` (see
[Configuration](../../../configuration.md)) to make it fail instead, with an
error that names the holder. Set `application_name` in each node's `dataSource`
to its pod name so the log names the holding pod. The state and statement time
of a session of another role show as `unknown` unless the waiting role has
`pg_read_all_stats`. On SQLite a second process on the same file waits up to
`busy_timeout`.

Step 1 of every store creates its tables if they are missing. On the vault and
the KVS it also converts an older layout: a Postgres vault `pkey` column of type
`TEXT` becomes `BYTEA`, and a primary key of `(pkey, ns)` becomes `(ns, pkey)`.
On a large vault this rewrites the table under an exclusive lock and can take a
long time.

A vault migrates when its channel opens, after the web listener has started.
While it does, `/healthz` answers 200 and `/readyz` answers 503 with reason
`migrating`, so a load balancer sends the node no traffic and a liveness probe
on `/healthz` does not restart it. The view stores migrate before the listener
starts; their step 1 is short unless a large KVS needs its key reordered. For
that case, give the first start a startup probe with enough time, or run
`node migrate` first.

Back up the database before the first start. A converted `TEXT` vault must never
be used by an FSC version that stores `pkey` as `TEXT` (see the warning in
[Configuration](../../../configuration.md)), and rolling back across step 1
means restoring the backup. After that, rolling a binary back to an earlier
release is free until a contract step has run (see
[Contract steps](#contract-steps)): a binary refuses only a store whose
`readable_by` is above the highest step it knows.

## Running migrations separately

Set `skipCreateTable: true` on the persistence and run `node migrate` from a Job
or init container with the same `core.yaml` and key material as the node:

```bash
./mynode node migrate --check || ./mynode node migrate
```

`node migrate` exits 0 when every store is current (or, without `--check`, has
been migrated), 10 when `--check` finds pending steps, 11 when a store was
migrated by a newer binary, 12 when `--contract` finds a store opened by a
binary without a pending contract step, 1 when it could not check or migrate a
store, and 2 on invalid arguments. `--check` opens every store and lists each
pending one with its steps, for example
`store [fsc_kvs] is at version 0, binary expects 1; pending: step 1 [create table]`.
A store that names no persistence while `fsc.persistences.default` is not
defined fails the command: it would use memory, and nothing would be migrated.

With `skipCreateTable: true` the node runs no step. An FSC store with pending
steps fails to open with `schema migration pending`, naming the store; this also
applies to stores opened after startup, such as a channel added at runtime.
`node migrate` reaches the stores the SDKs register at install time, which are
every view store, the Fabric endorse tx, metadata and envelope stores, and the
vault of every configured channel.

## Before rolling out a new binary

Run both checks with the new binary, the node's `core.yaml` and its key
material, before any node switches to it:

```bash
./mynode node migrate --check   # 10: steps pending, 11: the database is newer than this binary
./mynode node peers --check     # 11: a peer could not talk to this binary, 1: a peer is unreachable
```

`node version --json` lists the steps of every store the binary's SDKs register,
with their hashes, without a configuration. See
[Node CLI](../../../reference/node-cli.md).

## Database roles on Postgres

Use two roles. The migration role owns the FSC tables, because step 1 runs
`CREATE INDEX` and may run `ALTER TABLE` on them, and it creates
`<prefix>_schema_versions` and `<prefix>_schema_openers`. The node role needs
DML on the FSC tables, read access to the versions table, and `SELECT`, `INSERT`
and `UPDATE` on the openers table, where every open records the node with an
upsert; it runs with `skipCreateTable: true`:

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

Run the last block after `node migrate` has created `fsc_schema_versions` and
`fsc_schema_openers`; before that the `REVOKE` statements fail.

Whoever can write the versions table decides which steps a node skips, so keep
write access to it with the migration role.

## When a node refuses to start

| Error                                                                                                              | Cause                                                                          | Action                                                                                               |
| ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------- |
| `store [...] is at version N, binary expects M; pending: step N [...]: schema migration pending; run node migrate` | `skipCreateTable: true` and the store has steps pending                        | Run `node migrate`, then start the node.                                                             |
| `store [...]: migration lock on [...] not granted within ...; held by pid N (...)`                                 | `fsc.storage.migrationLockTimeout` elapsed while another session held the lock | See [A node waits for the migration lock](#a-node-waits-for-the-migration-lock).                     |
| `store [...]: failed to record opener in [<prefix>_schema_openers]: ... permission denied`                         | The node role cannot write the openers table                                   | Grant `SELECT, INSERT, UPDATE` on it; see [Database roles on Postgres](#database-roles-on-postgres). |
| `store [...] needs version N, binary knows M: schema is newer than this binary supports`                           | A newer binary ran contract step N, which this binary lacks                    | Run a binary that knows step N, or restore a backup taken before the contract step.                  |
| `store [...]: step N [...] failed`                                                                                 | A step failed and rolled back; earlier steps stay applied                      | Fix the cause in the wrapped error (often a missing privilege) and start again.                      |
| `table [...] primary key is (...), expected (ns, pkey)`                                                            | The table has a layout step 1 does not convert                                 | Fix the table by hand; step 1 left it unchanged.                                                     |
| `state table [...] stores pkey as [...], expected [bytea]`                                                         | The vault `pkey` column has a type other than `TEXT` or `BYTEA`                | Fix the column by hand.                                                                              |

## A node waits for the migration lock

A waiting node logs the holder's `pid` and `application_name` every 10 seconds.
A holder in state `active` whose statement keeps running is a step at work, such
as a vault rewrite; let it finish. A holder in state `idle` holds the lock
without migrating, for example a process that hangs after a step. Look at it,
then end its session; the lock is released with the session, and the step it was
running rolls back:

```sql
SELECT pid, application_name, state, now() - query_start AS running_for, query
FROM pg_stat_activity WHERE pid = 4711;
SELECT pg_terminate_backend(4711);
```

## Which binaries use a store

Every open of a store, including one with `skipCreateTable: true`, records the
node's `fsc.id` and the highest step its binary knows in
`<prefix>_schema_openers` (see
[Database layout](database-layout.md#schema-openers-table)). Replicas of one
node share an `fsc.id`; replicas on different binaries keep a row each. A row
stays after its replicas have been upgraded, because a row cannot tell a stopped
binary from one that still runs. `node migrate --contract` reads the table
before each contract step.

## Contract steps

Most steps only add tables, columns or indexes, and binaries without them keep
working on the store. A contract step drops or rewrites what such binaries read,
so it raises `readable_by` to its version and they refuse the store afterwards.

A node never runs a contract step when it starts, except on a new store: one
with neither a version row nor its primary table, such as in a new database.
Then it runs every step, unless an older binary opens the store meanwhile. A
store with tables but no version row, as v0.27 and earlier binaries leave it, is
not new. Otherwise the node runs the steps before the contract step, logs once
per open that the contract step is pending, and opens the store at its current
version; steps after the contract step wait too. `node migrate --check` lists
only the steps before the waiting contract step.
`node migrate --check --contract` also lists the contract step and those after
it, marked `(contract)`, and exits 12 if a binary that lacks the step opens the
store, so a pipeline learns before running `--contract` that it would be
refused. That check is a snapshot; `--contract` checks again under the lock.

To run it, once every replica of every node that opens the store runs a binary
that knows the step:

```bash
./mynode node migrate --contract
```

Under the migration lock, and in the contract step's transaction, the command
reads `<prefix>_schema_openers`. While a row of the store names a `highest_step`
below the step's version, it refuses with exit code 12 and lists each such node,
the step it knows and when it last opened the store. The rows of upgraded
binaries stay, so after the last replica of a node is upgraded, forget the
node's rows in the same run:

```bash
./mynode node migrate --contract --forget-opener node1 --forget-opener node2
```

> [!CAUTION]
> `--forget-opener` deletes every row of that `fsc.id`, including
> those of binaries that know the step; they are recorded again at their next
> open. Forget a node only when none of its replicas runs an older binary: a
> replica that keeps running on the store is not refused until it opens the
> store again, and then reads a store it no longer understands until it does.

A binary that opens the store while the step runs waits for it, then refuses the
store and leaves no opener row. On Postgres the step locks the openers table of
the whole table prefix, so every open of a store with that prefix, by any
binary, waits until the step commits, and a node role's `lock_timeout` or
`statement_timeout` can fail it; on SQLite an open waits at most for the
connection's `busy_timeout`. Keep contract steps short, or run them while no
replica restarts.

A binary of v0.27 or earlier records no opener row, so the command cannot see
it. Startup does not run a contract step on a store such a binary created, but
`--contract` does. Before the first contract step of a store, make sure no node
runs such a binary.

A contract step that fails rolls back with its version and `readable_by`, so
older binaries keep working. Once it has run, rolling a binary back below it
means restoring a backup.

FSC's own contract steps:

| Store                                                | Step                          | What it does                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| ---------------------------------------------------- | ----------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Endorse tx (`fsc_default_etx` by default)            | 2 `transactions as envelopes` | Rewrites every stored JSON transaction as a protobuf `TransactionEnvelope`, 1000 rows at a time in key order, all in the step's transaction, and only where the envelope reads back as the same transaction. That comparison ignores what an envelope does not keep by design: the proposal (`TProposal`), which receivers rebuild from the signed proposal; nil against empty for `RWSet`, `TParameters` and `TTransient`; and the channel key, which the envelope's routing fields carry. Rows that do not convert stay as they are and are counted in one warning. A transaction that is not wrapped in a Fabric `SerializedTransaction` and carries no channel key, as Fabric-x nodes of v0.27 write them, stays JSON: the store holds the transactions of every network of the node and cannot tell the platform of such a row. The warning counts these rows apart as "no channel key". A row whose key names a platform the binary has no transaction format for, such as a Fabric-x row in a binary built without the Fabric-x platform, stays JSON as well and is counted apart as "no format registered for their platform". A row that changed after the step read it keeps what was written. Afterwards the store writes envelopes. Binaries that read only JSON refuse the store. |
| Vault (`fsc_<network>__<channel>_vstate` by default) | 2 `metadata as protobuf`      | Rewrites the gob metadata of every state as a protobuf `VaultMetadata`, 1000 rows at a time in `(ns, pkey)` order, all in the step's transaction, and only where the protobuf reads back as the same map. Rows that do not convert stay gob and are counted in one warning. An empty map converts too and reads back as an empty map. A row whose metadata changed after the step read it, such as by a node that keeps running, keeps what that node wrote. Afterwards the vault writes protobuf. Binaries that read only gob refuse the vault.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |

Both stores read the old and the new form, before and after their step, and keep
reading the old form: rows the step leaves unconverted stay readable, and so
does what a replica writes before it notices the step. A store learns that its
step has run when it opens (see `migrate.ReadableByObserver` in
[Schema versions](db-driver.md#schema-versions)), so a replica that keeps
running across `node migrate --contract` writes the old form until it reopens
the store. Restart every replica after the step, one at a time, so that each
writes the new form.

Each step rewrites its table in one transaction, under the migration lock. While
it runs, the node's own writes to the rows it has rewritten wait for it to
commit, and, on Postgres, every open of a store under the same table prefix
waits too (see above). On a large endorse-tx store or vault, run
`node migrate --contract` in a maintenance window.
