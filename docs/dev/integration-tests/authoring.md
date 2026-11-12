# Authoring integration tests

Integration tests live in `integration/<platform>/<test-name>/` and use Ginkgo
v2 + Gomega. [Integration Tests](README.md) explains how suites, `make` targets
and CI jobs relate and how to run them; tests that inject faults are covered in
[Fault-Recovery Tests](fault-recovery.md).

## Adding a new test

1. Create the directory `integration/<platform>/<test-name>/`.
2. Define the topology, a function returning `[]api.Topology`, in `topology.go`.
   A topology that other suites share lives in
   `integration/<platform>/common/<name>/topology.go` instead, and the suite
   directory has none (`fabric/iou` uses the one in
   `integration/fabric/common/iou`).
3. Implement the test logic in `<test-name>_test.go`; it passes the topology to
   `integration.Generate` or `integration.GenerateAt`.
4. Add the Ginkgo suite entry point in `<test-name>_suite_test.go`, with
   `var _ = AfterSuite(integration.CleanupBuildArtifacts)` and the suite's
   `StartPort()` helper — see below.
5. Register the target in the `Makefile` — see below.

A suite directory holds only its own `_test.go` files plus code no other suite
imports. Topologies, views, states and helpers that several suites use live in
`integration/fabric/common/<name>/` or `integration/fabricx/common/<name>/`, so
a change made for one suite does not silently alter another. For example, the
IOU views in `integration/fabric/common/iou` back `fabric/iou`, `fabric/iouhsm`,
`fabric/configupdate`, `fabric/runtimeconfig`, `fabricx/iou` and
`fabricx/configupdate`.

### Deleting the built binaries

nwo builds `fsccli` and every FSC node binary into one temporary directory per
test process. `Infrastructure.Stop` keeps them, so the next network of the
process reuses them, and `go build` relinks only the ones whose sources changed
(nwo regenerates each node's `main` package from the topology).
`integration.CleanupBuildArtifacts` deletes the directory; without it in
`AfterSuite`, the binaries (about 100 MB each with `-race`) stay in the temp
directory after the suite exits.

### Registering the target

Appending the target to `INTEGRATION_TARGETS` in the
[`Makefile`](../../../Makefile) is enough when the name follows the
`<platform>-<suite>` convention, which maps to `integration/<platform>/<suite>`.
Otherwise:

- **Name does not match the directory** → set `INTEGRATION_DIR_<target>`. The
  target then shares the suite's ports, containers and `out/` with the suite's
  other targets, so those targets cannot run at the same time; see
  [One target at a time](../development.md#one-target-at-a-time).
- **The suite needs ginkgo flags** (e.g. a `--label-filter` so one suite becomes
  several parallel CI entries) → set `INTEGRATION_FLAGS_<target>`.
- **The suite needs a build tag** (e.g. `-tags pkcs11`) → set
  `INTEGRATION_GOFLAGS_<target>`; it becomes the `GOFLAGS` the test binary is
  built with.
- **The suite is too costly for every pull request** → add it to
  `NIGHTLY_INTEGRATION_TARGETS`; CI runs it from a scheduled workflow of its
  own.

## Nodes of another release

A node or replica can run a binary built at another FSC release instead of the
current tree, to test that releases work together:

- `ii.FscPlatform.SetRelease(id, release)`, called in the suite's generator
  after `integration.Generate` and before the network starts, selects the
  release of a node (all its replicas) or of one replica such as `alice.1`.
- `ii.UpgradeFSCNode(id, release)` stops a running replica and starts it on the
  binary of `release`, keeping its configuration and databases, as an operator's
  binary swap does. An empty `release` is the current tree.

`release` is a module version (a tag such as `v0.28.0`, or a commit) or the
absolute path of a checkout of the repository. nwo copies the node's generated
`main` into a module of its own that requires the FSC modules at `release`, and
builds it there without the race detector or coverage; a version needs the Go
module proxy or network access. The generated `main` imports the packages of
every view and SDK the topology registers for the node, so those packages must
exist, with the same API, at `release`. A suite that runs nodes of the previous
release therefore keeps its views in a package of its own that changes only
compatibly, and fails with a clear message when `fsc.ReleaseHasPackage` reports
that the release predates the package. `integration/fabric/mixedversion` runs at
`FSC_PREVIOUS_RELEASE`, which the `Makefile` sets to the latest release tag; set
it to a checkout to run the suite against local changes. Its specs that the
current tree refuses run at `FSC_OLDER_RELEASE`, which the `Makefile` sets to
v0.27.0, the last release without version negotiation; they use the views of
`integration/fsc/pingpong`, which exist at that release.

## Network topology options

There is no shared `integration.Opts`. Each suite declares its own `Opts` in its
`topology.go`, carrying only the fields that suite varies; `ReplicationOptions`
is the part the framework provides (`integration/utils.go`):

```go
// integration/<platform>/<test-name>/topology.go
type Opts struct {
    ReplicationOpts *integration.ReplicationOptions
    TLSEnabled      bool
}
```

The suite's `_test.go` then fills it in. A suite that also runs on libp2p adds a
`CommType` field (see [Choosing a p2p comm type](#choosing-a-p2p-comm-type)):

```go
opts := &Opts{
    TLSEnabled: true,
    ReplicationOpts: &integration.ReplicationOptions{
        ReplicationFactors: map[string]int{"node": 3},
        SQLConfigs:         map[string]*postgres.ContainerConfig{ /* ... */ },
    },
}
```

### Storage options

A node listed in `SQLConfigs` keeps all its stores in a Postgres default
persistence. Any other node keeps them in its own SQLite file, `default.sqlite`.
The vault is the exception: it goes in `fabric_vault.sqlite`, so `DeleteVault`
can remove it without touching the other stores. nwo starts each Postgres
database once, however many nodes or persistences share it. Use `NodeOptions` to
adjust a node's storage with the options below from `integration/nwo/fabric`.
`WithTablePrefix` and `WithPostgresTLS` fail unless an earlier option defined
the persistence they name. `WithStorePersistence` accepts any name, so a
misspelled one fails only when the node starts:

| Option                               | Effect                                                                                                             |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------ |
| `WithPostgresPersistence(name, cfg)` | defines another Postgres persistence, on the database of `cfg`                                                     |
| `WithStorePersistence(key, name)`    | moves one store there; `key` is `fsc_<store>` (`fsc_kvs`, `fsc_binding`, …) or `fabric_vault`                      |
| `WithTablePrefix(name, prefix)`      | renders `tablePrefix`, so the persistence's tables are named `<prefix>_…`                                          |
| `WithPostgresTLS(name)`              | starts the database with a generated CA and server certificate, and the node connects with `ssl_mode: verify-full` |

```go
NodeOptions: map[string][]node.Option{
    "borrower": {
        fabric.WithTablePrefix(common.DefaultPersistence, "borrower"),
        fabric.WithPostgresPersistence("kvs", postgres.DefaultConfig(postgres.WithDBName("borrower-kvs-db"))),
        fabric.WithStorePersistence("fsc_kvs", "kvs"),
    },
    "lender": {fabric.WithPostgresTLS(common.DefaultPersistence)},
},
```

The replicas variant of `integration/fabric/iou` exercises all four and checks
the resulting tables with `iou.Tables` (`integration/fabric/common/iou`). A unit
test that needs only a Postgres container with TLS can call `postgres.EnableTLS`
on its `ContainerConfig` before `postgres.StartPostgres`.

### Raising the p2p message-size limits

`fscTopology.SetP2PMessageSizeLimits(recv, send)` sets `fsc.p2p.maxRecvMsgSize`
and `fsc.p2p.maxSendMsgSize` (bytes) in the `core.yaml` of every FSC node in the
topology:

```go
fscTopology := fsc.NewTopology()
fscTopology.SetP2PMessageSizeLimits(20<<20, 20<<20) // recv, send
```

A zero or negative value leaves that key out, so the node default applies; with
neither set, the generated `core.yaml` has no size keys. Use it instead of
replacing `fscTopology.Templates.Core`: a copied template keeps emitting
configuration keys that FSC later renames or removes. The setting is
topology-wide and applies to every node.

### Renewing a node's TLS certificate

The node option `fsc.WithTLSReloadInterval(d)` sets `fsc.tls.reloadInterval`, so
the node's gRPC, web and metrics listeners re-read their certificate and key
every `d` and serve a renewed pair without a restart. Without it the key is left
out of `core.yaml` and the node never re-reads them.
`fscTopology.EnablePrometheusMetrics()` exports `fsc_tls_cert_not_after`, the
expiry of the certificate each listener serves, which `fsc.ParseTLSCertNotAfter`
reads by `listener` label.

The FSC platform renews a node's certificate on disk:

| Call                               | Effect                                                                                                                                                                                                                |
| ---------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `NodeTLSPair(peer)`                | reads the certificate and key the node's listeners serve                                                                                                                                                              |
| `RenewNodeTLSPair(peer, notAfter)` | returns a new pair, with a new key, signed by the TLS CA of the node's organization; it keeps the subject and names of the current certificate, so every client that verifies the node today verifies the renewed one |
| `WriteNodeTLSPair(peer, pair)`     | overwrites the node's certificate and key                                                                                                                                                                             |

`fsc/pingpong`'s "serves a renewed TLS certificate without a restart" renews the
initiator's certificate, checks that the gauge of both listeners moves, that a
new client still calls a view and that the initiator still reaches the responder
over P2P, then writes a mismatched pair and checks the renewed one stays, and
last writes the original pair back. Only the gRPC, web and metrics listeners
reload: the WebSocket P2P transport keeps the pair it loaded at startup,
`fsc.identity`'s, so the P2P ping is a liveness check and does not use the
renewed certificate. A client built before the renewal trusts the certificate
the node served then, and a client built from a mismatched pair on disk cannot
load it, so build each client for the check with the pair the node is expected
to serve.

### Choosing a p2p comm type

New suites use `fsc.WebSocket`. Add a `fsc.LibP2P` variant only when the suite
exercises something transport-specific that the host conformance tests in
`platform/view/services/comm` cannot reach — every extra comm type costs at
least one more network bootstrap, and one per spec unless the `Describe` is
`Ordered` (see [one network per `Describe`](#one-network-per-describe)).

Label each `Describe` with its comm type so a suite can be run one transport at
a time (`ginkgo --label-filter=libp2p`) and split into parallel CI entries:

```go
for _, c := range []fsc.P2PCommunicationType{fsc.WebSocket, fsc.LibP2P} {
	Describe("My Life Cycle", Label(c), func() { /* ... */ })
}
```

A WebSocket-only suite leaves `fscTopology.P2PCommunicationType` at its default,
`fsc.WebSocket`, and installs its SDK with `fscTopology.AddSDK(&SDK{})`.

`nwo` does not link the libp2p host. A suite that also runs on libp2p takes the
comm type as a parameter, sets it on the topology, and installs
`libp2psupport.SDK` (`integration/nwo/fsc/support/libp2p`) on its nodes:

```go
fscTopology.P2PCommunicationType = commType
fscTopology.AddSDKWithExtensions(&SDK{}, &libp2psupport.SDK{})
```

The field decides the transport: nwo writes it to each node's `fsc.p2p.type` and
generates the endpoints for it. `libp2psupport.SDK` only makes the libp2p host
available; it picks the host from `fsc.p2p.type` and uses the default WebSocket
host otherwise, so the same topology serves both comm types.

A suite that runs one comm type writes the `Describe` once, as
`Describe("My Life Cycle", Label(fsc.WebSocket), func() { /* ... */ })`. Label
WebSocket-only variants (e.g. replication) the same way.

> [!WARNING]
> `--label-filter` selects only labeled specs, so an unlabeled
> `Describe` is silently skipped when a filter is in play.

Label a `Describe` for the transport it _runs_, not the one it was handed. A
suite that loads committed artifacts runs whatever those artifacts say, because
`fsc.Platform.Load` is a no-op and never rewrites `core.yaml` —
`integration/fsc/pingpong` keeps its loading specs in one libp2p-labeled
`Describe` for exactly this reason. A label that lies puts specs in the wrong CI
job.

### Choosing the Fabric consensus

`fabric.NewDefaultTopology()` and `fabric.NewTopologyWithName` declare one
etcdraft orderer, `orderer`. To add orderers, replace `Orderers` and list the
same names in every profile. Keep one named `orderer`: nwo joins peers and
deploys chaincode through it. `integration/fabric/faultrecovery` runs three raft
orderers this way.

A SmartBFT network sets `Consensus.Type` to `"BFT"`, declares at least four
orderers (the minimum FSC's BFT broadcaster accepts) and adds the `V3_0` channel
capability to every profile; `Profile.SmartBFT` overrides configtxgen's
consensus defaults. Network `beta` of `integration/fabric/twonets/topology.go`
is one. An FSC node picks its broadcaster from the channel configuration, so the
topology is all a suite changes.

To check that a node used the BFT broadcaster, count its `fsc_connection_state`
series for the network's `orderer` component after one commit. The BFT
broadcaster sends the envelope to every orderer, so it leaves one series per
orderer. It returns once enough orderers accepted the envelope, so the last
series may appear after the commit: poll for the count with `Eventually`. The
CFT broadcaster sends it to one orderer per attempt, so it leaves one, or one
per retry.

## Declaring a namespace

A namespace is a chaincode plus its endorsement policy:

```go
fabricTopology.AddNamespace("iou", topology.Unanimity("Org1"))
```

The policy constructors are `Unanimity(orgs...)`, `OneOutOfN(orgs...)` and
`Signature(rule)`. Peers are derived from the policy's organizations; a
`Signature` policy names none, so it defaults to every peer on the channel.

By default a namespace deploys the built-in base chaincode as a container. To
deploy your own chaincode, name its image:

```go
fabricTopology.AddNamespace("events",
    topology.Signature(`OR ('Org1MSP.member','Org2MSP.member')`),
    topology.WithContainerImage("fsc-cc/events:latest"),
    topology.WithPeers("org1_peer", "org2_peer"),
)
```

Add the image to `scripts/chaincode/images.txt` and run `make chaincode-images`.
The chaincode's `main` must start a `shim.ChaincodeServer` reading
`CHAINCODE_ID` and `CHAINCODE_SERVER_ADDRESS`; copy one of the existing mains.

To have the peer build your chaincode from Go source instead, use
`WithLegacyChaincode(importPath)`. That path needs a `shim.Start` main and the
`ccenv` and `baseos` images (`make pull-images-fabric`) -- peers compile the
chaincode in the first and run it in the second. `integration/fabric/atsa`
(topology in `integration/fabric/common/atsa`) is the in-tree example, and the
only suite CI pulls those images for.

The remaining options are `WithCtor(ctor)` (which also turns on init),
`WithVersion(v)` and `WithPackageFile(path)` for a package you built yourself.

## Test utilities

- `integration.Infrastructure` — network lifecycle management.
- `integration.TestSuite` — base suite with setup/teardown.
- `StartPort()` — the first port of the suite's network. It is not part of
  `integration`: each suite defines it in `<test-name>_suite_test.go` as
  `integration.<Range>.StartPortForNode()`, on a `TestPortRange` of its own
  declared in `integration/ports.go`, so suites do not collide.

## Integration test structure

```go
var _ = Describe("Feature", Ordered, func() {
    s := NewTestSuite(opts...)
    BeforeAll(s.Setup)
    AfterAll(s.TearDown)
    It("creates and updates an IOU", s.TestCreateAndUpdate)
    It("rejects an update without the approver", s.TestUpdateWithoutApprover)
})
```

- Name each `It` after the behavior it checks: CI reports a failure as
  `<Describe> <It>`, so `It("succeeded")` says nothing about what broke.
- Define the network topology in `<test>/topology.go` via
  `integration.Generate()`.
- Put reusable test logic in `<test>/<test>.go`.
- For multi-node scenarios, use `integration.ReplicationOptions`.

### One network per `Describe`

`Setup` generates or loads artifacts and starts every node; `TearDown` stops
them. Under `BeforeEach`/`AfterEach` that cycle runs **per `It`** — roughly
90-110s for a Fabric topology, against 10-20s for the assertions inside it. A
five-spec `Describe` then spends most of its runtime booting the same network
five times.

So default to `Ordered` with `BeforeAll`/`AfterAll` and share one network across
the group. Reach for `BeforeEach` only when a spec genuinely needs a pristine
one: it stops a node and does not restart it, changes node configuration, or
writes state a later spec would read. The faultrecovery suites show the shared
form for specs that stop and restart nodes (`faultrecovery.ShareNetwork`, see
[Fault-Recovery Tests](fault-recovery.md#sharing-one-network)). A spec that
needs a _different_ topology belongs in its own `Describe` regardless — the
topology is fixed when `NewTestSuite` is called.

Specs that could share a network but would collide on fixed data (the same asset
ID, say) either parameterize that data or stay in separate `Describe`s.

## Focused runs (never commit)

Use `FIt`/`FDescribe` to focus a spec or `XIt` to skip one while iterating —
never commit them (CI treats a stray focus as a failure). Prefer the
`GINKGO_TEST_OPTS="--focus=..."` flag for anything you might commit.

Check that a focused run actually ran something. `go test` prints nothing for a
passing package, so a filter matching **zero specs** reports `ok ... 9.5s`,
which reads exactly like success. Pass `-v` and read `Ran N of M Specs` before
believing it.

## Gomega assertions in spawned goroutines

`gomega.Expect(...).To(...)` calls `ginkgo.Fail`, which panics to unwind to a
`recover()` that only exists on the goroutine Ginkgo is currently running the
spec on. A `go func(){ gomega.Expect(err).NotTo(gomega.HaveOccurred()) }()` has
no such `recover()`: a failed assertion there panics the whole test binary
instead of failing one spec.

- If the goroutine's result is joined before the spec ends
  (`sync.WaitGroup.Wait()`, `errgroup.Wait()`), `defer ginkgo.GinkgoRecover()`
  as its first statement turns that panic back into a normal spec failure.
- If the goroutine can outlive the spec (a fire-and-forget log streamer), don't
  put a `gomega.Expect` in it at all, even behind `GinkgoRecover` — Ginkgo would
  attribute a late panic to whichever spec happens to be running by then. Do the
  fallible call before spawning the goroutine and assert its error synchronously
  on the calling goroutine; let the goroutine itself only log-and-continue on
  errors that occur purely within its own work. See `docker.StartLogs`
  (`integration/nwo/common/docker/utils.go`).
