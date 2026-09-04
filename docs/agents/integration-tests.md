# Authoring Integration Tests

Integration tests live in `integration/<platform>/<test-name>/` and use
Ginkgo v2 + Gomega.

## Adding a new test

1. Create the directory `integration/<platform>/<test-name>/`.
2. Define the topology in `topology.go` (via `integration.Generate()`).
3. Implement the test logic in `<test-name>.go`.
4. Add the Ginkgo suite entry point in `<test-name>_test.go`.
5. Register the target in the `Makefile` — see below.

### Registering the target

Appending the target to `INTEGRATION_TARGETS` in the [`Makefile`](../../Makefile) is
enough when the name follows the `<platform>-<suite>` convention, which maps to
`integration/<platform>/<suite>`. Otherwise:

- **Name does not match the directory** → set `INTEGRATION_DIR_<target>`.
- **The suite needs ginkgo flags** (e.g. a `--label-filter` so one suite becomes several
  parallel CI entries) → set `INTEGRATION_FLAGS_<target>`.
- **The suite needs `-tags pkcs11`** → add it to `HSM_INTEGRATION_TARGETS` instead of
  `INTEGRATION_TARGETS`; that list compiles the test binary with the build tag.

## Network topology options

There is no shared `integration.Opts`. Each suite declares its own `Opts` in its
`topology.go`, carrying only the fields that suite varies; `ReplicationOptions` is the
part the framework provides (`integration/utils.go`):

```go
// integration/<platform>/<test-name>/topology.go
type Opts struct {
    CommType        fsc.P2PCommunicationType
    ReplicationOpts *integration.ReplicationOptions
    TLSEnabled      bool
}
```

The suite's `_test.go` then fills it in. `fsc.WebSocket` is the default comm type; use
`fsc.LibP2P` only with a reason:

```go
opts := &Opts{
    CommType:   fsc.WebSocket,
    TLSEnabled: true,
    ReplicationOpts: &integration.ReplicationOptions{
        ReplicationFactors: map[string]int{"node": 3},
        SQLConfigs:         map[string]*postgres.ContainerConfig{ /* ... */ },
    },
}
```

### Choosing a p2p comm type

New suites use `fsc.WebSocket`. Add a `fsc.LibP2P` variant only when the suite
exercises something transport-specific that the host conformance tests in
`platform/view/services/comm` cannot reach — every extra comm type costs a full
network bootstrap per spec, because each `It` re-runs `Setup`/`TearDown`.

Label each `Describe` with its comm type so a suite can be run one transport at
a time (`ginkgo --label-filter=libp2p`) and split into parallel CI entries:

    for _, c := range []fsc.P2PCommunicationType{fsc.WebSocket} {
        Describe("My Life Cycle", Label(c), func() { /* ... */ })
    }

Label websocket-only variants (replication, no-TLS) with `Label(fsc.WebSocket)`
too: `--label-filter` selects only labelled specs, so an unlabelled `Describe`
is silently skipped when a filter is in play.

## Declaring a namespace

A namespace is a chaincode plus its endorsement policy:

```go
fabricTopology.AddNamespace("iou", topology.Unanimity("Org1"))
```

The policy constructors are `Unanimity(orgs...)`, `OneOutOfN(orgs...)` and
`Signature(rule)`. Peers are derived from the policy's organizations; a
`Signature` policy names none, so it defaults to every peer on the channel.

By default a namespace deploys the built-in base chaincode as a container.
To deploy your own chaincode, name its image:

```go
fabricTopology.AddNamespace("events",
    topology.Signature(`OR ('Org1MSP.member','Org2MSP.member')`),
    topology.WithContainerImage("fsc-cc/events:latest"),
    topology.WithPeers("org1_peer", "org2_peer"),
)
```

Add the image to `scripts/chaincode/images.txt` and run `make chaincode-images`.
The chaincode's `main` must start a `shim.ChaincodeServer` reading `CHAINCODE_ID`
and `CHAINCODE_SERVER_ADDRESS`; copy one of the existing mains.

To have the peer build your chaincode from Go source instead, use
`WithLegacyChaincode(importPath)`. That path needs the `ccenv` image and a
`shim.Start` main. `integration/fabric/atsa` is the in-tree example.

The remaining options are `WithCtor(ctor)` (which also turns on init),
`WithVersion(v)` and `WithPackageFile(path)` for a package you built yourself.

## Test utilities

- `integration.Infrastructure` — network lifecycle management.
- `integration.TestSuite` — base suite with setup/teardown.
- `StartPort()` — dynamic port allocation to avoid collisions.

## Running

```bash
make list-integration-tests                             # every target
make integration-tests-fabric-iou                       # one target
GINKGO_TEST_OPTS="--focus='IOU Life Cycle'" make integration-tests-fabric-iou
```

A target's platform prefix decides which toolchain it needs: `fabric-*` targets
need `make install-fabric-bins` (binaries in `$FAB_BINS`), `fabricx-*` targets
need `make install-fabricx-tools` (tools in `$FAB_BINS/fabric-x`, kept apart
because both toolchains ship a `configtxgen`). Installing both is safe, and
`fsc-*` targets need neither.

Prerequisites (Fabric binaries, Docker images) and the remaining variants are in
[`docs/dev/development.md`](../dev/development.md#integration-tests).
