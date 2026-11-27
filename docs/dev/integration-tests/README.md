# Integration tests

An integration test starts a real network and drives FSC nodes through it: the
Fabric peers and orderers or the Chaguar committer, the FSC nodes built from
the test's topology, and any Postgres databases they use. The harness that does
this is `nwo`, in `integration/nwo`. Each suite lives in
`integration/<platform>/<suite>/`, where `<platform>` is `fsc` (FSC nodes only),
`fabric` or `fabricx`, and is written with Ginkgo v2 and Gomega.

| Page                                                              | Read it to                                                                                      |
| ----------------------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| [Authoring Integration Tests](authoring.md)                       | write a new suite: topology, storage, comm type, consensus, namespaces, spec structure          |
| [Chaguar committers in integration tests](fabricx-committers.md)  | configure the committer and orderer containers of a Chaguar network, or add committers          |
| [Fault-Recovery Tests](fault-recovery.md)                         | inject faults into a running network and check that FSC recovers                                |
| [Development Guide](../development.md#integration-tests)          | install the Fabric binaries and Docker images, and run, profile or measure coverage of a target |

## Suites and targets

Each suite has a `make` target named `integration-tests-<platform>-<suite>`.
Some suites are split by `Describe` label into several targets, which share the
suite's directory and network definition (see
[Registering the target](authoring.md#registering-the-target)).

| Suite                      | Targets                                                                                    | Covers                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| -------------------------- | ------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `fsc/pingpong`             | `fsc-pingpong`                                                                             | two FSC nodes without a ledger: network-based and node-based ping pong, generated and loaded artifacts, replicas                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `fsc/signedpingpong`       | `fsc-signedpingpong`                                                                       | ping pong whose messages are signed and verified with the identities of the session's parties                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `fsc/stoprestart`          | `fsc-stoprestart`                                                                          | a view call answered after the responder restarts                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `fabric/atsa`              | `fabric-atsa`                                                                              | asset transfer with FSC approvers, with and without replicas                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `fabric/atsachaincode`     | `fabric-atsachaincode`                                                                     | asset transfer through chaincode                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `fabric/configupdate`      | `fabric-configupdate`                                                                      | channel configuration updates                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `fabric/endorsers`         | `fabric-endorsers`                                                                         | an app that spreads its transactions over three endorser nodes of one organization, any one of which satisfies the endorsement policy, and moves to the next one when an endorser is killed, drains or hangs, after which the endorser it gave up on still ends with the transaction `Valid`                                                                                                                                                                                                                                                                                                                                                          |
| `fabric/events`            | `fabric-events`                                                                            | chaincode events                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `fabric/faultrecovery`     | `fabric-faultrecovery`, `-crash`, `-postgres-outage`, `-replication`, `-partition`, `-bft` | see [Fault-Recovery Tests](fault-recovery.md)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `fabric/iou`               | `fabric-iou`, `fabric-iou-libp2p`                                                          | the IOU life cycle, with replicas, on WebSocket and on libp2p                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `fabric/iouhsm`            | `fabric-iouhsm`                                                                            | the IOU life cycle with keys in an HSM (built with `-tags pkcs11`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `fabric/mixedversion`      | `fabric-mixedversion` (nightly)                                                            | nodes of the previous FSC release next to nodes of the current tree: endorsement across releases, a node upgraded and rolled back replica by replica on a shared database, a replica killed after broadcasting whose transaction the replica of the other release settles, a responder behind a load balancer upgraded and rolled back replica by replica while transactions run, and the contract steps the previous release lacks run with `node migrate --contract`; also nodes of a release without version negotiation, which the current tree refuses in both directions; see [Nodes of another release](authoring.md#nodes-of-another-release) |
| `fabric/mixednets`         | `fabric-mixednets`                                                                         | a Fabric and a Chaguar network side by side: a responder endorses a transaction for its non-default platform                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `fabric/runtimeconfig`     | `fabric-runtimeconfig`                                                                     | IOU reconfiguration while the node runs                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `fabric/twonets`           | `fabric-twonets`                                                                           | two Fabric networks: `alpha` on raft without TLS, `beta` on SmartBFT with TLS                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `fabricx/atsa`             | `fabricx-atsa`                                                                             | asset transfer on Chaguar                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `fabricx/configupdate`     | `fabricx-configupdate`                                                                     | channel configuration updates, and a refused configuration                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `fabricx/deployment`       | `fabricx-deployment`                                                                       | a namespace endorsement policy, enforced before and after an update                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `fabricx/endorsers`        | `fabricx-endorsers`                                                                        | an app that spreads its transactions over three endorser nodes of one organization, any one of which satisfies the endorsement policy, and moves to the next one when an endorser is killed, drains or hangs, after which the endorser it gave up on still ends with the transaction `Valid`                                                                                                                                                                                                                                                                                                                                                          |
| `fabricx/faultrecovery`    | `fabricx-faultrecovery`, `-crash`, `-replication`, `-partition`, `-committer-failover`     | see [Fault-Recovery Tests](fault-recovery.md)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `fabricx/iou`              | `fabricx-iou`                                                                              | the IOU life cycle on Chaguar                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `fabricx/multiendorsement` | `fabricx-multiendorsement`                                                                 | endorsements from several approvers merged into one transaction                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `fabricx/simple`           | `fabricx-simple`                                                                           | creating and querying objects, rejecting duplicates and negative values                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |

`make list-integration-tests` prints every target. To see the specs a target
runs without starting a network, run `ginkgo --dry-run -v` in the suite
directory with the target's `INTEGRATION_FLAGS_<target>` from the `Makefile`.

## Running a target

```bash
make integration-tests-fabric-iou
GINKGO_TEST_OPTS="--focus='IOU Life Cycle'" make integration-tests-fabric-iou
```

Run one target at a time: concurrent targets share ports, containers and `out/`,
and fail (see [One target at a time](../development.md#one-target-at-a-time)). A
target's platform prefix decides which toolchain it needs: `fabric-*` targets
need `make install-fabric-bins` (binaries in `$FAB_BINS`), `fabricx-*` targets
need `make install-fabricx-tools` (tools in `$FAB_BINS/fabric-x`, kept apart
because both toolchains ship a `configtxgen`). Installing both is safe, and
`fsc-*` targets need neither.

A filter that matches no spec still reports `ok`, so check the
`Ran N of M Specs` line with `-v` before taking a focused run as evidence.

## In CI

CI runs every target as a job of its own, on its own runner, from the list
`make list-integration-tests` prints; adding a target to the `Makefile` adds the
job. The jobs restore a shared Go build cache, so the time a suite adds to CI is
mostly its network startup and its specs. A suite that grows too long for one
job splits into labeled targets, as the fault-recovery suites do.

Targets in `NIGHTLY_INTEGRATION_TARGETS` are left out of that list and run on a
schedule instead. `fabric-mixedversion` builds a second set of node binaries, at
the previous release, so the `Mixed versions` workflow runs it nightly, on
demand, and on pull requests that change the suite, the release build, or code
two releases share: the SQL stores of the view and Fabric platforms and their
schema steps, the stream preamble, and the protobuf messages of
`platform/fabric/protos` and `platform/fabricx/protos`. A failed nightly run
opens the issue `Mixed versions nightly run failed`, labeled `testing` and `ci`,
or comments on it if it is open; the next successful nightly run closes it. A
run started by hand with the `report` input opens, comments on and closes the
issue the same way.

## Stuck specs and timeouts

Without `-v`, Ginkgo holds back a spec's output until the spec ends, so a hung
spec would print nothing. Every target therefore runs with these defaults, in
CI and locally:

- **Progress reports.** Once a spec node (`BeforeEach`, `It`, ...) has run for 5
  minutes, Ginkgo prints a progress report for it, and repeats it every minute.
  A report names the spec, the node and the current `By` step, shows the last
  10 lines the spec wrote to `GinkgoWriter`, and then the goroutine stacks,
  starting with the spec's. The slowest healthy node takes about 3 minutes, so
  a passing run normally prints none.
- **Suite timeout.** When a target has run for 20 minutes, counted from before
  the test binary is compiled, Ginkgo fails the running spec, prints its
  progress report and runs the cleanup nodes. It leaves a hung node running in
  the background: at once if the node takes no `SpecContext`, after 30 seconds
  (`--grace-period`) if it does. Each cleanup node gets the same 30 seconds.
  `fabric-mixedversion`, which also builds the previous release, gets 40
  minutes from `INTEGRATION_TIMEOUT_<target>` in the `Makefile`.
- **Job timeout.** The `itest` jobs stop after 30 minutes, which leaves room for
  set-up after the 20-minute suite timeout. It catches hangs Ginkgo can't see,
  such as a stuck `docker` call before the suite starts. The `Mixed versions`
  job stops after 60 minutes.

`GINKGO_TEST_OPTS` comes after these defaults on the `ginkgo` command line, so
its flags win. To debug with breakpoints, turn the suite timeout off:

```bash
GINKGO_TEST_OPTS="--timeout=0" make integration-tests-fsc-pingpong
```

To get a progress report from a running suite on demand, send `SIGUSR1` to the
test binary, which Ginkgo builds as `<suite directory>.test`, or press Ctrl+T on
macOS. The `ginkgo` command itself ignores `SIGUSR1`.

```bash
pkill -USR1 -f '/pingpong\.test'
```

## Code the suites share

A suite directory holds only its own `_test.go` files plus code no other suite
imports. Topologies, views and helpers that several suites use live in
`integration/fabric/common/<name>/` or `integration/fabricx/common/<name>/`.
Downstream projects such as Panurus import `integration`, `integration/nwo/...`
and `integration/benchmark/...`, so changes there must stay compatible; the
suite packages and their `common` packages are FSC's own.
