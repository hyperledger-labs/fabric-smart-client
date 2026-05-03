# Runtime DB Access

## Overview

This document describes how persistence-backed data is accessed while an FSC node is running.
It covers two different use cases:

- how application code should read or write persisted data from a running view
- how to inspect the live database of a running node for debugging or troubleshooting

The following principles apply throughout the runtime:

- FSC runtime data access is **service-oriented**. Application code is expected to use View, Fabric, or Fabric-x APIs instead of raw SQL connections.
- Not all runtime state is guaranteed to live in a local SQL database. This is especially relevant for Fabric-x, where state reads are backed by a query service.

Related documentation:

- [Database drivers](db-driver.md)
- [View platform configuration](../configuration.md)
- [Shared node configuration](../../../configuration.md)
- [Fabric platform](../../fabric/README.md)
- [Fabric-x platform](../../fabric-x/README.md)

## Runtime Access Paths

The following table summarizes the most common runtime access paths:

| Situation | Recommended entry point | Typical use case |
| --- | --- | --- |
| Writing a view that depends on persisted data | `view.Context.GetService(...)` | Access an application service that is backed by persistence |
| Reading Fabric state from a view | `state.GetVault(viewCtx)` | Read application state from a view |
| Working with lower-level Fabric vault functions | `fabric.GetDefaultChannel(viewCtx).Vault()` | Perform lower-level vault, query, or RWSet work |
| Reading Fabric-x state | `queryservice.GetQueryService(...)` or the channel API | Read state through the platform abstraction rather than local SQL |
| Inspecting a live node database | `sqlite3` or `psql` in read-only mode | Inspect persisted rows for debugging or troubleshooting |

## Runtime Access Layers

At runtime, persistence-backed data flows through the following layers:

```mermaid
flowchart LR
    App[Application view] --> Ctx[view.Context or platform API]
    Ctx --> Service[Runtime service]
    Service --> Store[Logical store]
    Store --> Persistence[Configured persistence]
    Persistence --> Backend[memory | sqlite | postgres]
```

These layers have different responsibilities:

- **Runtime service:** the API used by application code during view execution
- **Store:** the persistence-backed abstraction that owns queries and schema
- **Persistence:** the configured backend selection under `fsc.persistences`
- **Backend:** the actual database implementation, such as SQLite or Postgres

## Key Terms

For detailed definitions of persistence, store, and driver concepts, see the [Database drivers](db-driver.md) documentation.

Key runtime concepts:```

- **Persistence**: A named database configuration under `fsc.persistences` that defines the backend type and connection details
- **Store**: A logical runtime data partition (e.g., KVS, vault, envelope store) backed by one or more tables
- **Runtime Service**: The API that application code uses from a running view (e.g., `state.GetVault(viewCtx)`)
- **Direct DB Access**: Inspecting the underlying database with external tools like `sqlite3` or `psql` for troubleshooting

## Access from a Running View

### General Rule

Views and application services should prefer the runtime APIs exposed by FSC.
Normal application logic should not be built around direct SQL access.

This reflects the runtime model:

- the runtime already defines store interfaces and higher-level services
- some stores keep in-process caches
- Fabric and Fabric-x add their own abstractions on top of the underlying database
- not all runtime state is guaranteed to be local SQL state

### View Platform

The base View platform wires four persistence-backed stores:

- `KVS`
- `BindingStore`
- `SignerInfoStore`
- `AuditInfoStore`

They are created by the View SDK and backed by the configured persistences, but the default View SDK does not register all four as global services that every view can resolve.

In practice, this means:

- the View runtime uses these stores internally
- a custom application can expose additional storage-backed services to views through `GetService(...)`
- a plain view should not assume that a KVS instance or SQL handle is available through `GetService(...)` by default

If an application needs direct access to a persistence-backed store from a view, the recommended pattern is:

1. Wrap the store in a dedicated application service
2. Register that service in the application SDK or service registry
3. Retrieve the service through `view.Context.GetService(...)`

### Fabric Platform

The Fabric platform is the main place where runtime persisted state becomes an application-facing API.

There are two common access levels.

#### Simple Vault Access

Use `state.GetVault(viewCtx)` when you need the application-facing state service:

```go
func (v *MyView) Call(viewCtx view.Context) (any, error) {
    vault, err := state.GetVault(viewCtx)
    if err != nil {
        return nil, err
    }

    var myState MyState
    if err := vault.GetState(viewCtx.Context(), "mynamespace", "mykey", &myState); err != nil {
        return nil, err
    }

    return myState, nil
}
```

This is the simplest entry point for common Fabric state reads in views.

#### Advanced Channel and Vault Access

Use the channel API when you need lower-level control:

```go
func (v *MyView) Call(viewCtx view.Context) (any, error) {
    _, ch, err := fabric.GetDefaultChannel(viewCtx)
    if err != nil {
        return nil, err
    }

    vault := ch.Vault()
    qe, err := vault.NewQueryExecutor(viewCtx.Context())
    if err != nil {
        return nil, err
    }
    defer qe.Done()

    read, err := qe.GetState(viewCtx.Context(), "mynamespace", "mykey")
    if err != nil {
        return nil, err
    }

    return read, nil
}
```

The channel vault API is richer than the simple `state.Vault` helper.
It also exposes:

- transaction status
- read/write set creation
- read/write set inspection
- envelope storage
- transient data storage

### Fabric-x Platform

Fabric-x behaves differently from Fabric.
In practice:

- **Fabric:** runtime state reads are backed by the local vault store
- **Fabric-x:** runtime state reads are backed by a remote query service

This means that Fabric-x application code should treat runtime state access as a platform API. It should not assume that application state reads come from local SQL tables.

In particular:

- common entry points are `queryservice.GetQueryService(...)` and the channel/vault API
- the Fabric-x vault uses a `QueryService` for state reads
- range queries are not supported by the Fabric-x vault query executor
- local persistence still exists for envelope, metadata, and endorse-tx data, but Fabric-x state queries do not read from local SQL vault tables

Operationally:

- Fabric usually exposes runtime state through a local vault-backed service
- Fabric-x should be treated as a query-backed platform API, even if the node also persists some local data

## Inspecting a Live Node Database

### When Direct DB Access Makes Sense

Direct DB access is useful for:

- verifying that a node is persisting expected data
- checking transaction status during debugging
- inspecting whether state rows exist for a namespace or key
- comparing multiple nodes during troubleshooting

Direct inspection should be treated as **read-only**.

### Locate the Active Persistence

Each logical store chooses a persistence by name.
The persistence definitions live under `fsc.persistences`.

Common store-to-persistence mappings are:

- `fsc.kvs.persistence`
- `fsc.binding.persistence`
- `fsc.signerinfo.persistence`
- `fsc.auditinfo.persistence`
- `fsc.envelope.persistence`
- `fsc.metadata.persistence`
- `fsc.endorsetx.persistence`
- `fabric.<network>.vault.persistence`

Example:

```yaml
fsc:
  persistences:
    default:
      type: sqlite
      opts:
        dataSource: /var/lib/fsc/default.sqlite
        tablePrefix: fsc

  kvs:
    persistence: default
  binding:
    persistence: default
  signerinfo:
    persistence: default
  auditinfo:
    persistence: default
  envelope:
    persistence: default
  metadata:
    persistence: default
  endorsetx:
    persistence: default

fabric:
  mynetwork:
    vault:
      persistence: default
```

### Backend Types

The three supported backend types are `memory`, `sqlite`, and `postgres`. For detailed configuration options and behavior, see the [Database drivers](db-driver.md) and [Shared node configuration](../../../configuration.md) documentation.

At runtime:
- **memory**: in-memory SQLite, useful for tests but not inspectable after process exit
- **sqlite**: local file-based, easiest for local inspection with separate read/write handles
- **postgres**: external database, best for shared/remote inspection with a single connection pool

## Table Naming

Table names are derived from the configured `tablePrefix` (default: `fsc`) combined with store-specific suffixes and parameters. For complete details on table naming conventions and prefix configuration, see [Database drivers](db-driver.md).

### View Platform Tables

The KVS store is created without extra naming parameters, while the binding, signer info, and audit info stores are created with the `default` parameter.

With the default prefix, a generated node typically creates:

| Logical store | Default table | Main contents |
| --- | --- | --- |
| KVS | `fsc_kvs` | namespaced key/value data |
| Binding | `fsc_default_bind` | ephemeral-to-long-term identity bindings |
| Signer info | `fsc_default_sign` | known signer identities |
| Audit info | `fsc_default_aud` | audit bytes by identity |

### Fabric Platform Tables

The Fabric envelope, metadata, and endorse transaction stores also use the `default` parameter in a generated node.

With the default prefix, a generated node typically creates:

| Logical store | Default table | Main contents |
| --- | --- | --- |
| Envelope | `fsc_default_env` | envelope bytes by `network.channel.txid` |
| Metadata | `fsc_default_meta` | metadata bytes by `network.channel.txid` |
| EndorseTx | `fsc_default_etx` | endorse transaction bytes by `network.channel.txid` |

The Fabric vault tables are per network and per channel.
Their base suffixes are:

- `vstate`
- `vstatus`

So the effective table names are of the form:

- `<prefix>_<escaped network+channel params>_vstate`
- `<prefix>_<escaped network+channel params>_vstatus`

For a network named `default` and a channel named `testchannel`, the generated SQLite tables are:

- `fsc_default__testchannel_vstate`
- `fsc_default__testchannel_vstatus`

## What the Stored Data Looks Like

### View KVS

The View KVS stores:

- `ns`: logical namespace
- `pkey`: binary key
- `val`: raw value bytes

At the `kvs.KVS` API layer, values are typically JSON-marshaled before they are persisted.
This means the stored bytes are often JSON and may be readable after decoding, but the database still stores them as bytes.
In Fabric-based applications, the main application state is often stored in the Fabric vault instead of `fsc_kvs`, so it is normal for `fsc_kvs` to contain little or no application data.

### Binding, Signer, and Audit Stores

- binding rows map an ephemeral identity hash to a long-term identity
- signer rows track known identity hashes
- audit rows map identity hashes to audit bytes

### Fabric Envelope, Metadata, and EndorseTx Stores

These tables use a simple `key` / `data` layout.
The key is the logical transaction key:

`network.channel.txid`

The payload is opaque binary data from the point of view of direct DB inspection.

### Fabric Vault State and Status

The Fabric vault uses two main tables:

- `vstatus`
  - tracks transaction status codes and messages
- `vstate`
  - stores current state rows by namespace and key

The state rows include:

- namespace
- persisted key
- raw value bytes
- version bytes
- metadata bytes

For Postgres-backed vaults, namespace and key values are sanitized before storage.
For SQLite-backed vaults, no extra sanitizer is applied.

## Inspecting a Live Database

### Locating the Database

**SQLite**: The file path is defined in `fsc.persistences.<name>.opts.dataSource` (e.g., `/path/to/default.sqlite`)

**Postgres**: The connection string is defined in `fsc.persistences.<name>.opts.dataSource`

### Key Tables and Columns

**Transaction status** (`fsc_<network>__<channel>_vstatus`):
- `tx_id`, `code`, `message`, `pos`

**Vault state** (`fsc_<network>__<channel>_vstate`):
- `ns`, `pkey`, `val`, `metadata`, `version`

**Envelopes** (`fsc_default_env`):
- `key` (format: `network.channel.txid`), `data`

**View KVS** (`fsc_kvs`):
- `ns`, `pkey`, `val`

### Important Notes

- Payload columns (`val`, `data`, `metadata`) contain binary data that may be JSON, protobuf, or encoded bytes
- Use `hex()` or `length()` functions when inspecting binary columns
- Always use read-only access to avoid corrupting the database or invalidating caches

## What Is Safe at Runtime

### Safe

- reading tables directly for debugging
- checking whether expected rows were created
- comparing transaction status rows across nodes
- validating that a configured persistence is being used

### Unsafe or Unsupported

- modifying rows manually while the node is running
- deleting rows from the KVS or vault tables
- assuming that external DB writes will invalidate FSC caches
- assuming that Fabric-x runtime state is always available in a local SQL database

This matters because:

- the View KVS keeps an in-process cache
- the Fabric vault caches transaction status information
- Fabric-x keeps some runtime information in memory and reads state through the query service

For SQLite specifically, external live writes are also more likely to cause lock contention and `SQLITE_BUSY` style failures.
