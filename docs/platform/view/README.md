# View platform

The View platform is the core runtime of the Fabric Smart Client. It provides the communication, orchestration, identity, and service model that higher-level platforms build on.

## Start here

- [Programming model guide](programming-model.md): how to build an application around views, sessions and services
- [View API](view-api.md): the contracts of `view.View`, `view.Context`, `view.Session` and identities
- [View platform configuration](configuration.md)
- [View runtime security model](security-model.md)
- [View SDK architecture](view-sdk.md)

## Services

### Writing applications

- [View service](services/view-service.md): view lifecycle, contexts, sessions and responder dispatch
- [View service over HTTP](services/view-service-http.md)
- [Runtime DB access](services/runtime-db-access.md)

### Operating a node

- [Schema migrations](services/schema-migrations.md)
- [Database layout](services/database-layout.md)
- [Monitoring](services/monitoring.md)
- [Monitoring metrics](services/monitoring_metrics.md)
- [Shutdown](services/shutdown.md)

### Internals and extending

- [Configuration service](services/config-service.md)
- [Database drivers](services/db-driver.md)
- [Schema versioning](services/schema-versioning.md): adding versioned schema steps to a store
- [Communication services](services/comm/readme.md)

## Related documentation

- [Shared node configuration](../../configuration.md)
- [Fabric platform](../fabric/README.md)
