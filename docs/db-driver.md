# DB Drivers

## Driver

A driver corresponds to an implementation of a specific database.
A driver takes care of the connection to the DB and the instantiation of a logical store.
Usually each store corresponds to one table, but for normalization purposes it could also have multiple underlying tables.
Each store is defined with a separate method of the `Driver` interface.
Each store can also have references to other stores (e.g. foreign keys, joins).
Although these methods have different names and return different stores, their signatures have the same input parameters.

```go
type Driver interface {
    NewBindingStore(name PersistenceName, params ...string) (BindingStore, error)
    NewAuditInfoStore(name PersistenceName, params ...string) (AuditInfoStore, error)
	...
}

type BindingStore interface {
    GetLongTerm(ephemeral view.Identity) (view.Identity, error)
    HaveSameBinding(this, that view.Identity) (bool, error)
    PutBinding(ephemeral, longTerm view.Identity) error
}

type AuditInfoStore interface {
    GetAuditInfo(id view.Identity) ([]byte, error)
    PutAuditInfo(id view.Identity, info []byte) error
}
```

## Supported types

We currently support 3 database types:

* `memory.Driver`: Sqlite DB in memory
* `postgres.Driver`: Postgres DB via remote connection to standalone running DB (e.g. container)
* `sqlite.Driver`: Sqlite DB in the local filesystem

Aside to the database-related implementations of the `Driver` interface (i.e. `memory.Driver`, `postgres.Driver`, `sqlite.Driver`), there is a `multiplexer.Driver` that reads the underlying configuration and instantiates the store using the correct driver.

## Config

Each store has a dedicated configuration in the `core.yaml` file is read during the instantiation using the prefixed `Config` that is passed as a parameter.

This configuration might be at any level in the `core.yaml` file, but must have the following structure:

```yaml
key1:
  key2:
    type: sqlite
    opts:
      dataSource: /my/file/system
      maxOpenConns: 10
      maxIdleTimeout: 1m
```

The `Config` passed into the driver for the instantiation of a store, must be prefixed so that `Config.IsSet("type")` returns true.

## Extending the Driver

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
* add it into the config and make sure the `Config` passed is properly prefixed

## Adding Drivers

Although in the scope of this project, it is sufficient to extend the existing Driver with the addition of more methods for new stores, it will be necessary to add stores in extending projects.

In that case, you will need to create a new Driver interface that follows the same principles as before (containing only the new stores) and make sure that:
* the `Driver` is implemented by all necessary DBs. If you do not support `memory` in your `core.yaml`, then there is no need to create an extending `Driver`. Similarly, if you want to support more DBs, you will need to extend them accordingly.
* there is a new `multiplexer.Driver` that combines the rest of the `Driver` implementations
* the `Driver` implementations are wired in the SDK
* there is a section in the `core.yaml` that follows the same format and defines the characteristics of the store