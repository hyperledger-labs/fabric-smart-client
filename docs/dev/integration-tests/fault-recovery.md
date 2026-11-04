# Fault-Recovery Tests

The fault-recovery suites take parts of a running network down (FSC nodes, peers, orderers,
the Fabric-x committer, Postgres, network links) and check that FSC either recovers on its
own or fails within a bounded time with an error the caller sees. This page describes the
calls that inject faults, the two suites and their targets, the specs and helpers they
share, and the rules a spec on a shared network follows. How to create a suite in the
first place is in [Authoring Integration Tests](authoring.md).

## Fault primitives

nwo runs every FSC node, Fabric peer and Fabric orderer as its own process, so a
test can take one down while the rest of the network keeps running. Each call
returns once the component is gone or ready again, so no `time.Sleep` is needed
around it.

| Call | Effect |
|---|---|
| `StopFSCNode(id)` / `StartFSCNode(id)` | SIGTERM the node and wait for exit / start it again and wait for readiness |
| `KillFSCNode(id)` | SIGKILL the node, as a crash would; restart with `StartFSCNode` |
| `PauseFSCNode(id)` / `ResumeFSCNode(id)` | SIGSTOP / SIGCONT the node: it keeps its process and connections but falls behind, e.g. a replica while another commits |
| `MigrateFSCNode(id, flags...)` | run `node migrate` with flags such as `--check`, `--contract` or `--forget-opener` for a stopped node or replica |
| `StopPeer(name)` / `StartPeer(name)` | the same as `StopFSCNode` / `StartFSCNode` for a Fabric peer, e.g. `Org1_peer_0` |
| `StopOrderer(name)` / `StartOrderer(name)` | the same for a Fabric orderer, e.g. `orderer` |
| `StopPostgres(dbName)` / `StartPostgres(dbName)` | `docker stop` / `docker start` the Postgres container nwo started for `postgres.WithDBName(dbName)`; data and port survive |
| `fabricx.FxPlatform(ii).StopCommitter(names...)` / `StartCommitter(names...)` | stop / restart the services of the named Fabric-x committers, every committer with no name; see [Fabric-x committer](#fabric-x-committer) |
| `common.Proxy` `Partition()` / `Heal()` / `SetLatency(d)` | cut, restore or slow a link through a fault proxy; see [Partitions and latency](#partitions-and-latency) |
| `EventuallyConnectionState(node, conn, want, timeout)` | poll the node's `/metrics` until every `fsc_connection_state` series that `conn` (an `fsc.Connection`) selects is `want`; see [Connection state](#connection-state) |
| `WebClient(node).Ready()` | `nil` once the node's `GET /readyz` answers 200 |

The calls without a receiver are methods of `*integration.Infrastructure`.

Peer and orderer names must be unique across the Fabric networks of the topology;
the call fails otherwise. A Fabric topology may list several orderers (see
[Choosing the Fabric consensus](authoring.md#choosing-the-fabric-consensus)); every one of them joins
each channel at startup. Startup returns once each channel of an etcdraft network has a
raft leader (a SmartBFT channel has no leader election to wait for) and every peer
discovers all the others through gossip.

### Fabric-x committer

A Fabric-x network runs the mock orderer in a container of its own and each committer in
another, with its own database. The topology's `Committer` is the first committer;
`AddCommitter(org, name)` adds one for an organization, and each FSC node uses its own
organization's committers, or the first one when its organization has none.
`fabricx.FxPlatform(ii).StopCommitter(names...)` and `StartCommitter(names...)` stop and
restart the services (sidecar, coordinator, verifier, validator-committer, query) of the
named committers, every committer when no name is given. Their databases and the mock
orderer keep running. Start waits until the sidecar, query and ordering services accept
connections, and fails at once if the services exit first; a transaction ordered after that
commits. The mock orderer holds its blocks in memory only and loses them on restart, so a
Fabric-x spec cannot restart the orderer.

## The fault-recovery suites

Fault scenarios go in `integration/fabric/faultrecovery` and
`integration/fabricx/faultrecovery`. Each suite is split by `Describe` label into several
targets, which CI runs as parallel jobs:

| Target | Label | Network |
|---|---|---|
| `fabric-faultrecovery` | none of the others | the main network |
| `fabric-faultrecovery-crash` | `crash` | the main network |
| `fabric-faultrecovery-postgres-outage` | `postgres-outage` | the main network, with a shortened commit retry budget |
| `fabric-faultrecovery-replication` | `replication` | replicated nodes, see [Replicated nodes](#replicated-nodes) |
| `fabric-faultrecovery-partition` | `partition` | the main network behind fault proxies |
| `fabric-faultrecovery-bft` | `bft` | four SmartBFT orderers (`BFTTopology`) |
| `fabricx-faultrecovery` | none of the others | the main network |
| `fabricx-faultrecovery-crash` | `crash` | the main network |
| `fabricx-faultrecovery-replication` | `replication` | replicated nodes |
| `fabricx-faultrecovery-partition` | `partition` | the main network behind a fault proxy |
| `fabricx-faultrecovery-committer-per-org` | `committer-per-org` | a committer for each organization (`CommitterPerOrgTopology`) |

Both suites run the FSC nodes alice and bob on Postgres, and charlie, a second responder in
Org2, on SQLite, the default persistence. The Fabric suite adds three raft orderers, named
in its `Orderers`. Both Fabric topologies give Org2, bob's organization, the two peers in
`Org2Peers`. Bob's delivery reports each peer it has used as a `fsc_connection_state` series
of its own, and sets the one it left `disconnected`, so a spec that checks bob's delivery
after a peer fault looks at the one series that is not.

To list the scenarios a target runs, pass its label filter to a dry run, for example
`cd integration/fabric/faultrecovery && ginkgo --dry-run -v --label-filter=crash`.

### Sharing one network

The specs in the main `Describe` of each faultrecovery suite share one network
(`faultrecovery.ShareNetwork`, in an `Ordered, ContinueOnFailure` container), so the
network is set up once rather than per spec. A spec there must leave the network
healthy: restart every node it stops or kills, bring back every database, and leave the
node configuration alone. After a spec fails, the network is rebuilt before the next one.

A spec that passes must also leave no sessions or view contexts behind: after its own
`DeferCleanup`s, every FSC node and replica must hold, within `SessionCloseTimeout`, no more
`fsc_view_services_comm_sessions` and `fsc_view_services_view_contexts` than before it, or
the spec fails. A spec may restart a node it killed in a `DeferCleanup`.

A spec that changes the network for good may share it only as the last spec of an
`Ordered` container, which runs its specs in declaration order. Specs that change the
configuration before the network starts (such as `SetCommitRetries` or
`SetFinalityWaitTimeout`) go in a `Describe` of their own with its own generator, which
can again use `ShareNetwork`. To run that `Describe` as a parallel CI job, label it, give
the label its own target in the `Makefile`, and exclude the label in the default target's
negative filter. The Fabric suite's `postgres-outage` `Describe` and the
`fabric-faultrecovery-postgres-outage` target are the example; its failover spec is last
because it points bob at another database.

## Shared specs

What the two suites share lives in `integration/fabric/common/faultrecovery`: the nodes,
their views, the specs both suites run, and the helpers that drive them. A view or helper
both suites need goes there.

| Spec group | What it registers |
|---|---|
| `CrashSpecs(s, responder)` | the crash and catch-up specs, with `responder` as the node that crashes. They kill the responder after it endorsed, again while it catches up, and under load, and kill alice after she broadcast a transaction and before she saw it final. Each name ends in `(<responder>)` and carries the `crash` label; both suites call it for bob and for charlie, so a node on SQLite is crashed as well as one on Postgres. |
| `ClientSpecs(s)` | how alice's gRPC and web view clients behave across a restart of her node, and a call bob's responder is blocked in, cancelled through each client: the call fails with the cancellation and bob frees the responder's view context |
| `ViewSpecs(s)` | called once per suite: a view with two responders, bob and charlie, while charlie is down; it must fail within `SessionCloseTimeout`, and bob must release his half |
| `DependencySpecs(s, dbPrefix)` | take down bob's Postgres database, `DBName(dbPrefix, "bob")`, and bring it back |
| `HostileSpecs(s)` | a bob that changes the transaction before endorsing it; alice must reject the endorsement |
| `FailoverSpecs(s, conn)` | on `ReplicatedFSCTopology`: kill the replica of bob that holds alice's connection through bob's load balancer; `conn` is the connection a restarted replica must report `Connected` again before the next spec |

The `crash` specs run on the main network in a target of their own,
`fabric-faultrecovery-crash` and `fabricx-faultrecovery-crash`, which the default targets
exclude, so neither target outlasts CI's per-job limit.

## Helpers

| Helper | What it does |
|---|---|
| `SubmitAndWaitFinal(ii, n)` | runs `n` two-party transactions and waits until each is final on both nodes |
| `SubmitAndWaitFinalFrom` | the same from one of alice's replicas |
| `WaitFinal` / `WaitFinalOn` | the waiting alone, on both nodes / on one node or replica |
| `Endorse` / `Order` | split one transaction in two, so a spec can stop or kill a node after it has endorsed and before the transaction is ordered |
| `EndorseFrom` / `TryEndorseFrom` | alice collects from other responders, in turn; `TryEndorseFrom` returns the endorse view's error, so a spec can expect it to fail |
| `Broadcast` | runs alice's `broadcast` view, which orders a transaction without waiting for finality, so a spec can kill alice in between |
| `TrySubmitTo` | `TrySubmit` with given responders; the load spec runs workers that loop it, stops them before the kill, so each loses at most the call in flight, and restarts them once the responder endorses again |
| `OrderEventually` | retries `Order` while the orderers reject broadcasts, for example during a raft leader election |
| `EndorseEventually` | retries `Endorse` while alice's connection to bob fails over to another replica |
| `EndorseLiar` | runs alice's `endorseLiar` view against a bob that changes the transaction before endorsing it, and returns how alice rejected the endorsement |
| `Echo` | runs a two-party view that touches no ledger; with `block` set, bob's responder receives the message and never replies, so a spec can kill bob mid-view |
| `EchoTo` / `BlockedEcho` / `BlockedEchoWithContext` | echo to a given responder; `BlockedEcho` starts a blocking echo and returns once the responder blocks on a given node or replica; `BlockedEchoWithContext` ends its call with a context the spec controls |
| `AwaitEcho` | bob's responder waits for a second message that never comes, so a spec can kill alice mid-view |
| `MetricSum` | reads a counter or gauge from a node's metrics, such as `ViewContexts` or `Sessions` |
| `Query` / `ExpectSettled` | a node's status for a transaction and the value it wrote / wait until a node reads a transaction as valid with a given value and reports it final |
| `SetCommitRetries` / `SetFinalityWaitTimeout` | Fabric suite: shorten a node's commit retry budget before the network starts / how long `IsFinal` waits for a transaction that is not final; call them in the generator of a `Describe` of its own, since they apply to every spec that shares that generator |

On Fabric-x a node's vault is the committer's query service, so `Query` answers the same on
every node and does not show whether a restarted node has recovered.

Both nodes serve the `configseq` view, so a spec can change the channel configuration with
the `configupdate` helpers (`SetBatchTimeout`, `ConfigSequenceOn`, `ExpectConfigSequence`)
and check which configuration a node holds: `integration/fabric/common/configupdate` on
Fabric, and `integration/fabricx/common/configupdate` on Fabric-x, where `SetBatchTimeout`
takes the network from `NetworkOf` and a node applies a new configuration on its config
monitor's next poll of the committer.

### Postgres failover and vault rebuild

The helpers in `postgres.go` give a node's database a streaming-replication standby
(`StartStandby`, `WaitReplicated`, `KillPostgres`, `Promote`) and point a stopped node at
another `dataSource` (`DataSource`, `SetDataSource`), so a spec can fail a node over to a
new Postgres primary. `WipePostgres` empties a stopped node's database, so the node
replays the channel from the genesis block on its next start.

A Fabric node commits a replayed transaction it has no record of only if the transaction
touches a namespace registered with the committer's `ProcessNamespace` or another committer
rule selects it. The Fabric suite's `SDK` registers the faultrecovery namespace at startup;
since that also commits a transaction whose Busy record a node lost, only the `Describe` of
the wipe spec runs on it. Its `ContractProbeSDK`, which the `replication` `Describe` runs
on, adds a store with an expand and a contract step to replicas configured with
`SetContractProbeSteps(ii, replica, steps)`, so one binary stands for two releases on a
shared database.

## Partitions and latency

Killing a process makes the OS reset its connections, so the other side notices at once.
A network partition is silent. `common.Proxy.Partition()` reproduces it: the proxy stops
forwarding in both directions but closes nothing, and `Heal()` delivers the held bytes,
so every connection resumes where it stopped. `SetLatency(d)` delays every chunk the proxy
forwards by `d`.

**Fabric.** The Fabric suite's `partition` `Describe` calls `SetFaultProxies` in its
generator, which puts a proxy in front of alice's peer, bob's P2P port and bob's database,
and rewrites the `core.yaml` of both nodes to use them. FSC reads the ordering endpoints
from the channel configuration, not from `core.yaml`, so `SetFaultProxies` also puts a
proxy in front of each orderer, lists those proxies as the `OrdererEndpoints` in
`configtx.yaml` and builds the channel genesis block again; FSC nodes and peers then reach
the orderers through them, while raft traffic stays direct.

It also turns on alice's gRPC client keepalive (`KeepaliveTime`, 60 s, plus
`KeepaliveTimeout`, 10 s). nwo leaves gRPC keepalive off for every other node, and without
it a silent partition of the delivery peer is never detected. The websocket between two
FSC nodes has a keepalive of its own, on by default (`fsc.p2p.opts.websocket.keepalive`,
30 s plus 10 s), which closes a silently cut websocket and its sessions; `SetFaultProxies`
shortens it for alice and bob to `P2PKeepaliveInterval` plus `P2PKeepaliveTimeout`, 5 s
each, so a spec sees the cut well within a view's own receive timeout. Each spec there heals the proxy it partitions.

**Fabric-x.** The Fabric-x suite's `partition` `Describe` calls its own `SetFaultProxies`,
which puts a proxy in front of the committer's notification service and points bob's
`core.yaml` at it; bob's query service stays direct, since each query call is bounded by
its own request timeout. It turns on the keepalive of bob's notification service
(`notificationService.keepalive`, see [configuration](../../configuration.md)) with the
same 60 s and 10 s: the committer's sidecar rejects pings more often than every 60 s, and
the committer pushes nothing while no transaction is ordered, so the keepalive is what
detects a silently cut notification stream. `RewriteCore` and `StartProxy` in
`integration/fabric/common/faultrecovery` serve both suites.

## Replicated nodes

Replicas of a node share its name, and the other nodes use only the first resolver entry
for a name, so without a load balancer every session they open lands on replica 0. The
node option `fsc.WithLoadBalancer()` starts a round-robin TCP proxy (`common.Proxy`) in
front of the P2P ports of the node's replicas and writes the proxy's address as the node's
P2P address in every other node's `core.yaml`. The proxy forwards each new connection to
the next replica that accepts a dial and passes the bytes through, so TLS ends at the
replica; this works because nwo gives every replica of a node the same TLS key pair, which
the websocket transport requires since it authenticates both sides from the TLS
certificates. `FscPlatform.LoadBalancer(name).Connections()` returns the open connections
per replica P2P address, so a spec can find the replica that holds a peer's connection;
`faultrecovery.ConnectedReplica(ii, name)` returns that replica and the others.

`StopFSCNode`, `KillFSCNode` and `StartFSCNode` take a replica name such as `bob.1` to act
on that replica alone, and `Client("fsc.bob.1")` and `WebClient("fsc.bob.1")` reach it. A
node name selects the first replica in the fault calls and the last one in `Client` and
`WebClient`. Helpers that take a node name, such as `faultrecovery.WaitFinal`, therefore
reach its last replica; while a replica is down, poll the survivor by replica ID, for
example with `WaitFinalOn(ii, "fsc.bob.0", txID)`.

The `replication` `Describe` of each faultrecovery suite runs on
`faultrecovery.ReplicatedFSCTopology`: bob with two replicas behind a load balancer, alice
with two replicas without one, each node on its own database. On Fabric-x, the finality of
a transaction that is already final also comes from the query service, so a replica shows
that its own notification stream answers a subscription only by reporting a transaction
final whose finality view started before the transaction was ordered; the Fabric-x
suite's `expectStreamedFinality` does that on every bob replica.

## Connection state

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
[Connection State](../../platform/view/services/monitoring_metrics.md#connection-state).
