# Architecture and dependency injection

## Platform layers

```text
platform/
├── view/      # Core: P2P comm, view execution, identity, storage, metrics/tracing
├── fabric/    # Fabric integration: chaincode, endorsement, vault, RWSet
└── fabricx/   # Chaguar integration (same structure as fabric/)
```

Each platform has an `sdk/dig/sdk.go` that wires services with
[uber/dig](https://github.com/uber-go/dig). SDKs compose hierarchically:

```text
BaseSDK → ViewSDK → FabricSDK → FabricXSDK → Application SDK
```

A platform depends on the platforms below it, never on those built on it: no
package under `platform/fabric/` imports `platform/fabricx/`, and a test in
`platform/fabric/protos` checks it. Where Fabric code needs per-platform
behavior, the platform built on it registers it. The transaction formats are an
example: `platform/fabric/protos` defines the `TransactionEnvelope` and the
Fabric format, and `platform/fabricx/protos` registers the Chaguar format with
`RegisterTransactionFormat` in an `init` function.

## The DI Install/Start pattern

All services are registered and resolved through the DI container. `Install()`
registers providers and **must call the parent SDK's `Install()`**; `Start()`
resolves and starts services.

```go
func (p *SDK) Install() error {
    return errors.Join(
        p.Container().Provide(NewService),
        p.Container().Provide(NewDriver, dig.Group("drivers")),
        p.Container().Provide(NewProvider, dig.As(new(Interface))),
        p.SDK.Install(), // always call the parent
    )
}

func (p *SDK) Start(ctx context.Context) error {
    return p.Container().Invoke(func(svc *Service) error {
        return svc.Start(ctx)
    })
}
```

- Use `dig.In` structs to inject multiple dependencies; `dig.Group(...)` for
  collections (e.g. storage drivers register into the `db-drivers` group).
- Dependencies are resolved automatically via constructor injection.

## Lifecycle and shutdown

`pkg/node.Node` runs `Install`, then `Start(ctx)`, then `PostStart(ctx)` on
every SDK, in install order. `Node.Stop` reverses that
([shutdown.md](../platform/view/services/shutdown.md)):

1. It calls `Drain(ctx)` on the `node.Drainer` in the node's registry, bounded
   by `fsc.shutdown.drainTimeout`. The view SDK registers the view manager,
   which refuses new work and waits for running views.
2. It cancels the `ctx` passed to `Start`.
3. It calls `Stop()` on every SDK in reverse install order, bounded by
   `fsc.shutdown.teardownTimeout`.

So an SDK that wraps another stops its own components in `Stop()` and then calls
the wrapped SDK's `Stop()`: what it uses (the KVS, comm, the servers of the view
SDK) is still running while it stops. Do not tear down in a `ctx.Done()`
goroutine: nothing orders it against the other SDKs. The drain needs nothing
from an SDK; `Drain` is not part of `commondig.SDK`, so wrappers need not pass
it through.

## Listeners and view transports

The view SDK (`platform/view/sdk/dig`) owns the `fsc.grpc.enabled`,
`fsc.web.enabled` and `fsc.view.remote.enabled` flags: a disabled listener's
constructor is never called, whatever implementation is installed, and with the
remote view API disabled no backend is mounted.

**Replacing a listener.** The `fsc.grpc` and `fsc.web` listeners are the
`GRPCServer` and `WebServer` interfaces, built by a `GRPCServerConstructor` and
a `WebServerConstructor`. Decorate the constructor in a child SDK's `Install`,
after calling the parent. The decorator returns a constructor, not a server, so
the built-in listener is never built and never binds a port, and the SDK still
calls the replacement only when the listener is enabled:

```go
p.Container().Decorate(func(viewsdk.GRPCServerConstructor) viewsdk.GRPCServerConstructor {
    return mygrpc.New // func(driver.ConfigService) (viewsdk.GRPCServer, error)
})
```

A replacement must expose the connection's TLS state to handlers as the built-in
listeners do (the view service binds commands to a verified client certificate),
and `Stop` must wait for in-flight handlers. `SDK.Start` calls a `WebServer`'s
`Start` inline, so it must return once the listener serves, or with the error
that prevents it; a `GRPCServer`'s `Start` serves until `Stop`, in a goroutine
of its own.

**Adding a view transport.** The remote view API is mounted by `Backend`s,
collected from the `view-remote-backends` dig group. `SDK.Start` mounts the ones
`fsc.view.remote.backends` lists, in list order, and fails on a listed name no
backend has. Provide one and list its name:

```go
p.Container().Provide(newNATSBackend, dig.Group(viewsdk.BackendsGroup))
```

```yaml
fsc:
  view:
    remote:
      enabled: true
      backends: [grpc, nats]
```

The SDK constructs every backend in the group to read its name, so a backend's
constructor must be free of side effects; `Mount` acquires what the transport
needs and returns an error if a prerequisite, such as its listener, is missing.

**Restricting who calls which view.** Every command on the remote view API
passes the `server.PolicyChecker` the view SDK provides: the built-in signature
and client check, then every checker in the `fsc-view-policy-checkers` dig
group. Provide one from a child SDK's `Install`; a refusal wraps
`view.ErrDenied`. Details and an ACL example:
[View Service](../platform/view/services/view-service.md#remote-view-api-access-control).

```go
p.Container().Provide(func() viewsdk.PolicyCheckerEntry { return viewsdk.PolicyCheckerEntry{Checker: myACL} })
```

**Reporting application readiness.** `GET /readyz` answers 503 while a gate in
the `fsc-readiness-gates` dig group is not ready, with the gate's reason in the
body. The node's own gates, `draining`, `migrating` and `grpc server stopped`,
come first, and connection state comes after. A child SDK provides a gate from
its `Install` for startup work of its own, such as loading parameters or
rebuilding a store. `NotReady` runs on every probe and must not block.
`/healthz` and the gRPC health service ignore these gates.

```go
p.Container().Provide(func() viewsdk.ReadinessGateEntry { return viewsdk.ReadinessGateEntry{Gate: myGate} })
```

## Multi-network support

FSC can run against multiple Fabric networks simultaneously. Network names are
configured in `core.yaml`; access a specific network by name via
`FabricNetworkService(networkName)`.

## Fabric transaction patterns

Two patterns for assembling Fabric transactions:

- **Chaincode-mediated** — parties agree on input via views, one party invokes
  the chaincode, all wait for confirmation.
- **Approver-mediated** — parties assemble the RWSet directly, sign, and send to
  FSC-based approvers (who hold endorser keys) for submission to the ordering
  service.

Worked examples live in
[`docs/core-concepts.md`](../core-concepts.md#transaction-lifecycle-or-how-to-orchestrate-a-business-process).

## Where FSC sits in the ecosystem

- **[Panurus](https://github.com/LFDT-Panurus/panurus)** is a token layer built
  on top of FSC. It uses FSC's view/session model and FSC's `pkg/utils/errors`.
- **[Chaguar Committer](https://github.com/Chaguar/committer)** is
  the validation/commit service the `fabricx` platform submits transactions to.
  It is a separate Go codebase with its own conventions; FSC shares its wire
  protocol, not its Go style.
