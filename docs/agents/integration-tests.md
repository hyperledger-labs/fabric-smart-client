# Authoring Integration Tests

Integration tests live in `integration/<platform>/<test-name>/` and use
Ginkgo v2 + Gomega.

## Adding a new test

1. Create the directory `integration/<platform>/<test-name>/`.
2. Define the topology in `topology.go` (via `integration.Generate()`).
3. Implement the test logic in `<test-name>.go`.
4. Add the Ginkgo suite entry point in `<test-name>_suite_test.go`, with
   `var _ = AfterSuite(integration.CleanupBuildArtifacts)` — see below.
5. Register the target in the `Makefile` — see below.

### Deleting the built binaries

nwo builds `fsccli` and every FSC node binary into one temporary directory per test
process. `Infrastructure.Stop` keeps them, so the next network of the process reuses
them, and `go build` relinks only the ones whose sources changed (nwo regenerates each
node's `main` package from the topology). `integration.CleanupBuildArtifacts` deletes
the directory; without it in `AfterSuite`, the binaries (about 100 MB each with
`-race`) stay in the temp directory after the suite exits.

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

### Raising the p2p message-size limits

`fscTopology.SetP2PMessageSizeLimits(recv, send)` sets `fsc.p2p.maxRecvMsgSize` and
`fsc.p2p.maxSendMsgSize` (bytes) in the `core.yaml` of every FSC node in the topology:

```go
fscTopology := fsc.NewTopology()
fscTopology.SetP2PMessageSizeLimits(20<<20, 20<<20) // recv, send
```

A zero or negative value leaves that key out, so the node default applies; with neither
set, the generated `core.yaml` has no size keys. Use it instead of replacing
`fscTopology.Templates.Core`: a copied template keeps emitting configuration keys that
FSC later renames or removes. The setting is topology-wide and applies to every node.

### Choosing a p2p comm type

New suites use `fsc.WebSocket`. Add a `fsc.LibP2P` variant only when the suite
exercises something transport-specific that the host conformance tests in
`platform/view/services/comm` cannot reach — every extra comm type costs at least
one more network bootstrap, and one per spec unless the `Describe` is `Ordered`
(see [one network per `Describe`](testing.md#one-network-per-describe)).

Label each `Describe` with its comm type so a suite can be run one transport at
a time (`ginkgo --label-filter=libp2p`) and split into parallel CI entries:

    for _, c := range []fsc.P2PCommunicationType{fsc.WebSocket} {
        Describe("My Life Cycle", Label(c), func() { /* ... */ })
    }

Label websocket-only variants (replication, no-TLS) with `Label(fsc.WebSocket)`
too: `--label-filter` selects only labelled specs, so an unlabelled `Describe`
is silently skipped when a filter is in play.

Label a `Describe` for the transport it *runs*, not the one it was handed. A suite
that loads committed artifacts runs whatever those artifacts say, because
`fsc.Platform.Load` is a no-op and never rewrites `core.yaml` — `integration/fsc/pingpong`
keeps its loading specs in one libp2p-labelled `Describe` for exactly this reason.
A label that lies puts specs in the wrong CI job.

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
`WithLegacyChaincode(importPath)`. That path needs a `shim.Start` main and the
`ccenv` and `baseos` images (`make pull-images-fabric`) -- peers compile the
chaincode in the first and run it in the second. `integration/fabric/atsa` is
the in-tree example, and the only suite CI pulls those images for.

The remaining options are `WithCtor(ctor)` (which also turns on init),
`WithVersion(v)` and `WithPackageFile(path)` for a package you built yourself.

## Test utilities

- `integration.Infrastructure` — network lifecycle management.
- `integration.TestSuite` — base suite with setup/teardown.
- `StartPort()` — dynamic port allocation to avoid collisions.

### Fault primitives

nwo runs every FSC node, Fabric peer and Fabric orderer as its own process, so a
test can take one down while the rest of the network keeps running. Each call
returns once the component is gone or ready again, so no `time.Sleep` is needed
around it.

| Call on `*integration.Infrastructure` | Effect |
|---|---|
| `StopFSCNode(id)` / `StartFSCNode(id)` | SIGTERM the node and wait for exit / start it again and wait for readiness |
| `KillFSCNode(id)` | SIGKILL the node, as a crash would; restart with `StartFSCNode` |
| `StopPeer(name)` / `StartPeer(name)` | the same for a Fabric peer, e.g. `Org1_peer_0` |
| `StopOrderer(name)` / `StartOrderer(name)` | the same for a Fabric orderer, e.g. `orderer` |
| `StopPostgres(dbName)` / `StartPostgres(dbName)` | `docker stop` / `docker start` the Postgres container nwo started for `postgres.WithDBName(dbName)`; data and port survive |
| `EventuallyConnectionState(node, conn, want, timeout)` | poll the node's `/metrics` until every `fsc_connection_state` series that `conn` (an `fsc.Connection`) selects is `want`; see below |
| `WebClient(node).Ready()` | `nil` once the node's `GET /readyz` answers 200 |

Peer and orderer names must be unique across the Fabric networks of the topology;
the call fails otherwise. A Fabric topology may list several raft orderers; every one
of them joins each channel at startup. Startup returns once each channel of an
etcdraft network has a raft leader and every peer discovers all the others through
gossip. For Fabric-x,
`fabricx.FxPlatform(ii).StopCommitter()` and `StartCommitter()` stop and restart the
all-in-one committer container; start waits
until its sidecar, query and ordering services accept connections.

Fault scenarios go in `integration/fabric/faultrecovery` and
`integration/fabricx/faultrecovery` (targets `fabric-faultrecovery`,
`fabric-faultrecovery-postgres-outage` and `fabricx-faultrecovery`). Both run the FSC
nodes alice and bob on Postgres; the Fabric suite adds three raft orderers, named in its
`Orderers`. What the two suites share lives in `integration/fabric/common/faultrecovery`:
the nodes, their views, the specs both suites run (`CrashSpecs`, `DependencySpecs`), and the helpers that drive them.
`SubmitAndWaitFinal(ii, n)` runs `n` two-party transactions and waits until each is final
on both nodes; `WaitFinal` does the waiting alone. `Endorse` and `Order` split one
transaction in two, so a spec can stop or kill a node after it has endorsed and before the
transaction is ordered. `OrderEventually` retries `Order` while the orderers reject
broadcasts, for example during a raft leader election. `Echo` runs a two-party view that
touches no ledger; with `block` set, bob's responder receives the message and never
replies, so a spec can kill bob mid-view. `MetricSum` reads a counter or gauge from a
node's metrics. The Fabric suite's `SetCommitRetries` shortens a node's commit retry
budget before the network starts; call it in the generator of a `Describe` of its own, since
it applies to every spec that shares that generator. `Query` returns a node's
status for a transaction and the value it wrote. On Fabric-x a node's vault is the
committer's query service, so `Query` answers the same on every node and does not show
whether a restarted node has recovered. A view or helper both suites need goes there.
The helpers in `postgres.go` give a node's database a streaming-replication standby
(`StartStandby`, `WaitReplicated`, `KillPostgres`, `Promote`) and point a stopped node at
another `dataSource` (`DataSource`, `SetDataSource`), so a spec can fail a node over to a
new Postgres primary.

The specs in the main `Describe` of each faultrecovery suite share one network
(`faultrecovery.ShareNetwork`, in an `Ordered, ContinueOnFailure` container), so the
network is set up once rather than per spec. A spec there must leave the network
healthy: restart every node it stops or kills, bring back every database, and leave the
node configuration alone. After a spec fails, the network is rebuilt before the next one.
A spec that cannot restore the network, or that changes the configuration before the
network starts (such as `SetCommitRetries`), goes in a `Describe` that sets up its own
network per spec (`BeforeEach(s.Setup)`, `AfterEach(s.TearDown)`). To run that
`Describe` as a parallel CI job, label it, give the label its own target in the
`Makefile`, and exclude the label in the default target's negative filter. The Fabric
suite's `postgres-outage` label and the `fabric-faultrecovery-postgres-outage` target
are the example.

`fsc.Connection` holds the four labels of a series: `Network`, `Channel`, `Component`
and `Endpoint`. An empty field matches any value, and at least one series must match.
Leaving `Network` or `Channel` empty therefore requires the connections of every
network or channel to be in `want`, so one that is down cannot hide behind another:

```go
ii.EventuallyConnectionState("alice", fsc.Connection{Component: connstate.ComponentDelivery}, connstate.Connected, time.Minute)
```

The gauge holds what the node last observed, which can be stale. The `orderer` series
changes only when the node broadcasts: after `StopOrderer` it keeps its pre-stop value
until a broadcast fails, so asserting `Connected` right after the stop passes while the
orderer is down. Make the node broadcast before asserting a state that follows an
orderer fault. The states, the component names and when each component reports are
listed under
[Connection State](../platform/view/services/monitoring_metrics.md#connection-state).

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
