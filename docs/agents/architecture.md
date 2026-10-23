# Architecture & Dependency Injection

## Platform layers

```
platform/
├── view/      # Core: P2P comm, view execution, identity, storage, metrics/tracing
├── fabric/    # Fabric integration: chaincode, endorsement, vault, RWSet
└── fabricx/   # Fabric-x integration (same structure as fabric/)
```

Each platform has an `sdk/dig/sdk.go` that wires services with
[uber/dig](https://github.com/uber-go/dig). SDKs compose hierarchically:

```
BaseSDK → ViewSDK → FabricSDK → FabricXSDK → Application SDK
```

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

## Listeners and view transports

The view SDK (`platform/view/sdk/dig`) owns the `fsc.grpc.enabled`, `fsc.web.enabled` and
`fsc.view.remote.enabled` flags: a disabled listener's constructor is never called, whatever
implementation is installed, and with the remote view API disabled no backend is mounted.

**Replacing a listener.** The `fsc.grpc` and `fsc.web` listeners are the `GRPCServer` and
`WebServer` interfaces, built by a `GRPCServerConstructor` and a `WebServerConstructor`.
Decorate the constructor in a child SDK's `Install`, after calling the parent. The decorator
returns a constructor, not a server, so the built-in listener is never built and never binds
a port, and the SDK still calls the replacement only when the listener is enabled:

```go
p.Container().Decorate(func(viewsdk.GRPCServerConstructor) viewsdk.GRPCServerConstructor {
    return mygrpc.New // func(driver.ConfigService) (viewsdk.GRPCServer, error)
})
```

A replacement must expose the connection's TLS state to handlers as the built-in listeners do
(the view service binds commands to a verified client certificate), and `Stop` must wait for
in-flight handlers.

**Adding a view transport.** The remote view API is mounted by `Backend`s, collected from the
`view-remote-backends` dig group. `SDK.Start` mounts the ones `fsc.view.remote.backends`
lists, in list order, and fails on a listed name no backend has. Provide one and list its
name:

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

The SDK constructs every backend in the group to read its name, so a backend's constructor
must be free of side effects; `Mount` acquires what the transport needs and returns an error
if a prerequisite, such as its listener, is missing.

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

- **[Panurus](https://github.com/LFDT-Panurus/panurus)** (formerly Token SDK) is a
  token layer built *on top of* FSC. It reuses FSC's view/session model and even
  imports `pkg/utils/errors`, so its conventions are a stricter superset of FSC's —
  a useful reference when tightening our own.
- **[Fabric-x Committer](https://github.com/hyperledger/fabric-x-committer)** is the
  validation/commit service the `fabricx` platform submits transactions to. It is a
  separate Go codebase with deliberately different conventions (e.g. `cockroachdb/errors`,
  concrete-types-over-interfaces) — align with it only on the wire/protocol, not on Go style.
