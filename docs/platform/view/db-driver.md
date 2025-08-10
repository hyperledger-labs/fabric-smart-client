# DB Drivers

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

* Further properties such as `tablePrefix`, `maxIdleConns` defines the persistence.
Two persistences can share the same `type` and `dataSource`, but not the rest of the options.
In this case, separate connections will be opened for each persistence and different table prefixes will be used (if defined so).

We can have one or more persistences in a project, as well as one optional fallback persistence (`default`).

### Store
A store is a logical partition of the project storage, e.g. identity info, transactions, ledger state.
It consists of one or more underlying tables and exposes querying functionality through an interface.
A store can also have references to other stores (e.g. foreign keys, joins).
It has one implementation per persistence type, e.g.:
* `memory.BindingStore`: Sqlite DB in memory (in practice this does not exist, as the implementation is identical to the `sqlite.BindingStore`)
* `postgres.BindingStore`: Postgres DB via remote connection to standalone running DB (e.g. container)
* `sqlite.BindingStore`: Sqlite DB in the local filesystem

However, they all share one common interface such as the following:

```go
type BindingStore interface {
    GetLongTerm(ephemeral view.Identity) (view.Identity, error)
    HaveSameBinding(this, that view.Identity) (bool, error)
    PutBindings(longTerm view.Identity, ephemeral ...view.Identity) error
}

type AuditInfoStore interface {
    GetAuditInfo(ctx context.Context, id view.Identity) ([]byte, error)
    PutAuditInfo(ctx context.Context, id view.Identity, info []byte) error
}
```

### Driver
A driver is responsible for the instantiation of multiple (or usually all) stores.
It has multiple implementations: one for each persistence type.
* `memory.Driver`: Sqlite DB in memory
* `postgres.Driver`: Postgres DB via remote connection to standalone running DB (e.g. container)
* `sqlite.Driver`: Sqlite DB in the local filesystem

Aside to the database-related implementations of the `Driver` interface (i.e. `memory.Driver`, `postgres.Driver`, `sqlite.Driver`), there is a `multiplexer.Driver` that reads the underlying configuration and instantiates the store using the correct driver.

During instantiation, the driver has access to the persistence `opts`.
This way, it takes care of:
* opening the connection to the DB (unless we are re-using the same `dataSource`)
* the creation of the schema (unless it has already been created by another store before)
* the instantiation of the service that builds and executes the queries to the database
The driver interface of an application has the following format:

```go
type Driver interface {
    NewBindingStore(name PersistenceName, params ...string) (BindingStore, error)
    NewAuditInfoStore(name PersistenceName, params ...string) (AuditInfoStore, error)
	...
}
```

**Note:** Although these methods have different names and return different stores, their signatures have the same input parameters for the sake of uniformity, code re-use, and maintainability.


## Config

All persistences are defined under `fsc.persistences` in the `core.yaml`. Each persistence has a unique persistence name and there can be one `default` persistence.

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
      dataSource: dataSource1   # Reuses the same connection as default, but opens a new connection
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
If there is no `default`, an error is returned.

```yaml
fabric:
  my_network:
    metadata:
      persistence: my_custom_postgres
    audit:
      persistence:  # The default will be chosen
fsc:
  signer:
    persistence: my_custom_postgres # Will re-use the same persistence
```

The `Config` passed into the driver for the instantiation of a store, must be prefixed so that `Config.IsSet("type")` returns true.

## Extending the Driver on the same Project

If you want to add a new store (e.g. `TransactionStore`), you will need to:
* extend the `Driver` interface with one more method that has the same inputs
```go
type Driver interface {
    NewBindingStore(name PersistenceName, params ...string) (BindingStore, error)
    NewAuditInfoStore(name PersistenceName, params ...string) (AuditInfoStore, error)
    ...
	NewTransactionStore(name PersistenceName, params ...string) (TransactionStore, error)
}
```
* implement it for all 4 drivers (DB specific and multiplexed)

## Adding Drivers or Creating Drivers on new Projects

Although in the scope of this project, it is sufficient to extend the existing Driver with the addition of more methods for new stores, it will be necessary to add stores in extending projects.

In that case, you will need to create a new Driver interface that follows the same principles as before (containing only the new stores) and make sure that:
* the `Driver` is implemented by all necessary DBs. If you do not support `memory` in your `core.yaml`, then there is no need to create an extending `Driver`. Similarly, if you want to support more DBs, you will need to extend them accordingly.
* there is a new `multiplexer.Driver` that combines the rest of the `Driver` implementations
* the `Driver` implementations are wired in the SDK
* there is a section in the `core.yaml` that follows the same format and defines the characteristics of the store