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
