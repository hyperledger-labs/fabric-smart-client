# Fabric-x Platform Driver

> NOTE: The Fabric-x driver is **currently experimental** and under active development. 
> The code may be incomplete, contain bugs, or lack necessary security and performance optimizations. 
> It should not be used in any production environment until the final, stable release is made available.
> Contributions and feedback are highly encouraged!

## Overview

The **Fabric Smart Client (FSC)** includes support for [Fabric-x](https://github.com/hyperledger/fabric-x), available under [`platform/fabricx`](../../../platform/fabricx).

The Fabric-x platform driver currently builds upon the existing Fabric platform implementation and replaces only the components necessary for Fabric-x–specific functionality, such as the transaction processing component. 

## Application Node

To use the Fabric-x platform within an FSC application node, you need to install the Fabric-x [SDK](../../../platform/fabricx/sdk/dig/sdk.go):

```go
import (
	"log"

	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	fabricx "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

func main() {
	node := fscnode.New()
	if err := node.InstallSDK(fabricx.NewSDK(node)); err != nil {
		log.Fatalf("failed installing SDK: %v", err)
	}
	node.Execute(func() error {
		registry := view.GetRegistry(node)
		// register your views ...
		return nil
	})
}
```

### Configuration

Start with the [Fabric-x platform configuration](configuration.md) page for the documented `notificationService` and `queryService` settings, then layer those on top of the shared FSC [node configuration](../../configuration.md).

## Notification Service (Transaction Finality)

The notification service delivers realtime transaction finality events to FSC application nodes. It maintains a persistent bidirectional gRPC stream to the [fabric-x-committer](https://github.com/hyperledger/fabric-x-committer)'s `SidecarService` (`OpenNotificationStream`), which pushes notifications as transactions are committed, invalidated, or timed out. The connection to the committer is re-used and shared among all active listeners for a given network and channel.

### API

Register through the channel's committer — `ch.Committer()`, the same API every FSC driver exposes — and use:

- `AddFinalityListener(txID, listener)`: registers a `FinalityListener` whose `OnStatus` callback fires once when the transaction reaches finality. Only the first listener for a given txID triggers a subscription request to the committer; subsequent listeners for the same txID piggyback on it. After dispatch, all listeners for the txID are automatically removed. Registering the same listener instance twice for one txID is a no-op.
- `RemoveFinalityListener(txID, listener)`: unregisters a listener before delivery. Safe to call for unknown txIDs, unregistered listeners, and listeners already delivered to.

The `OnStatus` callback receives one of: `fdriver.Valid` (committed), `fdriver.Invalid` (rejected), or `fdriver.Unknown` (undetermined / timeout).

The committer's manager is the notification-stream manager wrapped in a `finality.ResolvingListenerManager`, which asks the query service before subscribing. That is what makes registering for an *already final* transaction report its status instead of `Unknown`: the notification stream reports each transaction once, so a subscription created after that point would never be answered. `finality.GetListenerManager(sp, network, channel)` returns the unwrapped manager and does not do this.

> [!WARNING]
> `OnStatus` implementations MUST observe `ctx.Done()` and return promptly. At most
> `handlerWorkers` (default 16) callbacks run at once, so one that blocks forever
> occupies a slot indefinitely — and once all slots are occupied, finality
> notifications stop being delivered. Hand slow work to your own queue and return.
> See [the notification service tuning
> guide](configuration.md#notification-service-tuning) for the full failure mode.

### Limitations

- **Reconnection keeps pending listeners**: when the stream breaks, the manager reopens it with jittered exponential backoff (100ms up to 10s) until the node shuts down. Listeners stay registered across the outage, and the new stream requests every pending txID again. `AddFinalityListener` during an outage succeeds; the transaction is requested once the stream is back. `listenerTTL` keeps running, so a listener waits at most `listenerTTL + sweepInterval` whether or not the stream recovers. Only node shutdown settles what is still pending, with `Unknown`. The committer reports only statuses that arrive after a subscription, so a transaction that committed while the stream was down is answered as timed out once `requestTimeout` passes; through the channel's committer that `Unknown` is re-resolved from the query service and the listener receives the real outcome.
- **Bounded handler concurrency**: at most `handlerWorkers` (default 16) callbacks run concurrently, buffered by a queue of `handlerQueueSize` (default 1000) so bursts larger than the limit are still delivered in full. `handlerTimeout` (default 5s) only cancels the callback's context — it cannot force a return, so a callback that ignores cancellation occupies its worker indefinitely. Once every worker is occupied and the queue fills, the transaction stays registered — together with the status the committer sent — and is retried on the next sweep, so delivery is delayed rather than lost. Queueing is all-or-nothing per transaction: either every listener waiting on it is handed to the pool, or none is, which is what keeps delivery exactly-once across a retry. The cost of never dropping a result is that the pending-listener map grows while the pool is congested; see [notification service tuning](configuration.md#notification-service-tuning).
- **Shutdown does not wait on stuck handlers**: when the node shuts down, everything still registered is queued for settlement, the queue is closed so the workers finish the backlog, and the wait is abandoned after a single `handlerTimeout` — one deadline for the whole teardown, not one per listener. A callback still inside an unresponsive listener can outlive the manager, so a single bad listener cannot hang node shutdown; at most `handlerWorkers` such callbacks exist, since each occupies one worker. Listeners queued behind one are not settled — nothing can force such a callback to return.

### Configuration

The notification service is configured under the `fabric.<network>.notificationService` key.

```yaml
fabric:
  <network>:
    notificationService:
      requestTimeout: 30s
      handlerWorkers: 16
      handlerQueueSize: 1000
      endpoints:
        - address: "committer.example.com:9090"
          connectionTimeout: 5s
          tls:
            enabled: true
            rootCerts:
              - "/path/to/ca.pem"
```

#### TLS Modes

**No TLS** — omit the `tls` block or set `enabled: false`:

```yaml
endpoints:
  - address: "committer.example.com:9090"
```

**Server TLS** — client verifies the server certificate against the provided root CAs:

```yaml
endpoints:
  - address: "committer.example.com:9090"
    tls:
      enabled: true
      rootCerts:
        - "/path/to/ca.pem"
```

**Mutual TLS (mTLS)** — both sides authenticate each other. Requires `clientCert` and `clientKey` in addition to `rootCerts`:

```yaml
endpoints:
  - address: "committer.example.com:9090"
    tls:
      enabled: true
      rootCerts:
        - "/path/to/ca.pem"
      clientCert: "/path/to/client.pem"
      clientKey: "/path/to/client-key.pem"
```

The `serverNameOverride` field can be set to override the TLS server name used for hostname verification, which is useful when connecting by IP address:

```yaml
tls:
  enabled: true
  rootCerts:
    - "/path/to/ca.pem"
  serverNameOverride: "committer.example.com"
```

### Usage from a View
Refer to the [Simple integration test fixtures](/integration/fabricx/common/simple) for reference.


## Query Service

The query service provides synchronous access to ledger state by connecting to the [fabric-x-committer](https://github.com/hyperledger/fabric-x-committer)'s `Query` service over gRPC. It is used for operations such as retrieving transaction status and channel configuration.

### Configuration

The query service is configured under the `fabric.<network>.queryService` key. It supports the same TLS modes as the notification service.

```yaml
fabric:
  <network>:
    queryService:
      requestTimeout: 30s
      endpoints:
        - address: "committer.example.com:9091"
          connectionTimeout: 5s
          tls:
            enabled: true
            rootCerts:
              - "/path/to/ca.pem"
```

For mTLS, add `clientCert` and `clientKey` under the `tls` block — see the [Notification Service TLS Modes](#tls-modes) section above for the full set of options, which apply identically here.


## Integration Tests

The integration tests for the Fabric-x platform use the [fabric-x-committer-test-node](https://github.com/hyperledger/fabric-x-committer/pkgs/container/fabric-x-committer-test-node) container image.
nwo starts it as:
- a mock ordering service, in a container of its own, reached as `orderer.localhost` (its alias on the suite's Docker network, and loopback on the host);
- a [committer](https://github.com/hyperledger/fabric-x-committer) for every organization with a peer, each in a container of its own with its own Postgres, reading the same blocks from the orderer.

Each committer is named `CommitterName(org)` (`Org1_committer_0`, after Fabric's `Org1_peer_0`), and each FSC node uses its own organization's committers.

### Committer Container Configuration

The committers are configured through the `nwofabricx.Topology` returned by `NewTopology()`,
`NewDefaultTopology()`, or `NewTopologyWithName()`. All options have sensible defaults so no additional
configuration is required for standard test scenarios: every organization with a peer gets a
committer, and the first organization's is the network's default committer.

#### Settings for every committer

| Method | Default | Description |
|---|---|---|
| `WithCommitterImage(image string)` | `hyperledger/fabric-x-committer-test-node:1.0.7` | Docker image for every committer container and the orderer container. |
| `WithCommitterEnv(key, value string)` | *(see below)* | Override or add an environment variable of every committer container. Pass an empty value to remove a default. |
| `WithOrdererEnv(key, value string)` | *(see below)* | Override or add an environment variable of the mock orderer container, such as `SC_ORDERER_BLOCK_SIZE`. Pass an empty value to remove a default. |

#### Adding committers

`AddCommitter(org string, opts ...CommitterOption)` adds a committer to `org`, such as a second one
in an organization. Each committer runs as a sidecar peer inside the Fabric network, with its own
container, database and ports. Options configure it:

| Option | Default | Description |
|---|---|---|
| `WithName(name string)` | next free `<org>_committer_<n>` | Peer name for the committer's sidecar identity in the Fabric network. |
| `WithHost(host string)` | *(auto)* | Fixed host for the sidecar peer entry; leave empty to use the Docker bridge IP. |
| `WithPorts(ports api.Ports)` | *(auto)* | Pre-allocated ports; leave nil to let the test framework allocate them. |
| `WithImage(image string)` | the topology's | Docker image for this committer's container. |
| `WithEnv(key, value string)` | the topology's | Override or add an environment variable of this committer's container. |

Default environment variables set by the test framework:

| Variable | Default value |
|---|---|
| `SC_SIDECAR_LOGGING_LOGSPEC` | `info:grpc=error` |
| `SC_QUERY_LOGGING_LOGSPEC` | `info:grpc=error` |
| `SC_COORDINATOR_LOGGING_LOGSPEC` | `info:grpc=error` |
| `SC_ORDERER_LOGGING_LOGSPEC` | `info:grpc=error` |
| `SC_VC_LOGGING_LOGSPEC` | `info:grpc=error` |
| `SC_VERIFIER_LOGGING_LOGSPEC` | `info:grpc=error` |
| `SC_ORDERER_BLOCK_SIZE` | `1` |
| `SC_SIDECAR_ORDERER_SIGNED_ENVELOPES` | `true` |
| `SC_SIDECAR_SERVER_MAX_CONCURRENT_STREAMS` | `0` |
| `SC_QUERY_SERVER_MAX_CONCURRENT_STREAMS` | `0` |
| Endpoints and MSP dirs | *(derived from network)* |

#### Example

```go
fxTopo := nwofabricx.NewDefaultTopology()
fxTopo.AddOrganizationsByName("Org1", "Org2") // runs Org1_committer_0 and Org2_committer_0

// run a committer image built locally from a fabric-x-committer checkout
fxTopo.WithCommitterImage("hyperledger/fabric-x-committer-test-node:local")

// increase the orderer's block size and turn on its debug logging
fxTopo.
    WithOrdererEnv("SC_ORDERER_BLOCK_SIZE", "10").
    WithOrdererEnv("SC_ORDERER_LOGGING_LOGSPEC", "debug")

// remove a default committer env var entirely, on every committer
fxTopo.WithCommitterEnv("SC_VC_LOGGING_LOGSPEC", "")

// add a second committer to Org1, Org1_committer_1, and a third one named and logging at debug
fxTopo.AddCommitter("Org1")
fxTopo.AddCommitter("Org1", nwofabricx.WithName("audit"), nwofabricx.WithEnv("SC_SIDECAR_LOGGING_LOGSPEC", "debug"))
```

### Example: Simple Fabric-x Network Topology

The example below defines a simple Fabric-x network with two organizations (`Org1` and `Org2`) and two FSC application nodes (`alice` and `bob`).

In this setup:
- `alice` acts as the endorser for the `iou` namespace.
- The namespace creation policy is controlled by Org1.

```go
import (
	fabricx "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
)

type Topology() []*api.Topology {
	fxTopo := nwofabricx.NewDefaultTopology()
	fxTopo.AddOrganizationsByName("Org1", "Org2")
	// set the namespace creation policy to Org1
	fxTopo.SetNamespaceApproverOrgs("Org1")
	// define a namespace named "iou" with Org1 as endorser/approver
	fxTopo.AddNamespace("iou", topology.Unanimity("Org1"))
	
	fscTopo := fsc.NewTopology()
	// register fabricx SDK
	fscTopo.AddSDK(fabricx.SDK{})
	// create alice and promote her as endorser
	fscTopo.AddNodeByName("alice").
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(scv2.WithApproverRole()).
		// register your views ...
	
	fscTopo := fsc.NewTopology()
	fscTopo.AddNodeByName("bob").
		AddOptions(fabric.WithOrganization("Org2")).
		// register your views ...
	
	return []api.Topology{fxTop, fscTopo}
}
```

More examples can be found in [`integration/fabricx/`](../../../integration/fabricx/).

## Additional Resources

- Fabric-x Repository: https://github.com/hyperledger/fabric-x
- Fabric-x Committer: https://github.com/hyperledger/fabric-x-committer
