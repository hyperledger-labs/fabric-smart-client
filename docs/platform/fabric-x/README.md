# Fabric-x Platform Driver

> NOTE: The Fabric-x driver is **currently experimental** and under active development. 
> The code may be incomplete, contain bugs, or lack necessary security and performance optimizations. 
> It should not be used in any production environment until the final, stable release is made available.
> Contributions and feedback are highly encouraged!

## Overview

The **Fabric Smart Client (FSC)** includes support for [Fabric-x](https://github.com/hyperledger/fabric-x), available under [`platform/fabricx`](../../../platform/fabricx).

The Fabric-x platform driver currently builds upon the existing Fabric platform implementation and replaces only the components necessary for Fabric-x–specific functionality, such as the transaction processing component. 

## Application Node

To use the Fabric-x platform within an FSC application node, you need to install the Fabric-x [SDK](`../../../platform/fabricx/sdk/dig/sdk.go`):

```go
import (
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	fabricx "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

func main() {
	node := fscnode.New()
	if err := node.InstallSDK(fabricx.NewSDK(node)); err != nil {
		panic(err)
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

The notification service delivers realtime transaction finality events to FSC application nodes. It maintains a persistent bidirectional gRPC stream to the [fabric-x-committer](https://github.com/hyperledger/fabric-x-committer)'s `Notifier` service, which pushes notifications as transactions are committed, invalidated, or timed out. The connection to the committer is re-used and shared among all active listeners for a given network and channel.

### API

Obtain a `ListenerManager` via `finality.GetListenerManager(sp, network, channel)`, then use:

- `AddFinalityListener(txID, listener)`: registers a `FinalityListener` whose `OnStatus` callback fires once when the transaction reaches finality. Only the first listener for a given txID triggers a subscription request to the committer; subsequent listeners for the same txID piggyback on it. After dispatch, all listeners for the txID are automatically removed.
- `RemoveFinalityListener(txID, listener)`: unregisters a listener before delivery. Safe to call for unknown txIDs or unregistered listeners.

The `OnStatus` callback receives one of: `fdriver.Valid` (committed), `fdriver.Invalid` (rejected), or `fdriver.Unknown` (undetermined / timeout).

### Limitations

- **No automatic reconnection**: if the stream breaks, the manager is removed and registered listeners are lost. A new manager is created on the next `GetListenerManager` call.
- **Handler timeout**: callbacks that exceed 5 seconds (`DefaultHandlerTimeout`) are abandoned with a warning. Handlers that ignore context cancellation will leak a goroutine but won't block other listeners.

### Configuration

The notification service is configured under the `fabric.<network>.notificationService` key.

```yaml
fabric:
  <network>:
    notificationService:
      requestTimeout: 30s
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
Refer to the [Simple integration test](/integration/fabricx/simple) for reference.


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
This container sets up a test networking including:
- An ordering service
- A [committer](https://github.com/hyperledger/fabric-x-committer) instance

### Committer Container Configuration

The committer container is configured through the `nwofabricx.Topology` returned by `NewTopology()`,
`NewDefaultTopology()`, or `NewTopologyWithName()`. All options have sensible defaults so no additional
configuration is required for standard test scenarios.

#### Sidecar peer identity

The committer runs as a sidecar peer inside the Fabric network. You can customise the peer name, its
organisation, and — when running in a non-standard network layout — its host and ports:

| Method | Default | Description |
|---|---|---|
| `WithCommitterName(name string)` | `"SC"` | Peer name for the committer sidecar identity in the Fabric network. |
| `WithCommitterOrg(org string)` | `"Org1"` | Organisation the committer peer belongs to. |
| `WithCommitterHost(host string)` | *(auto)* | Fixed host for the sidecar peer entry; leave empty to use the Docker bridge IP. |
| `WithCommitterPorts(ports api.Ports)` | *(auto)* | Pre-allocated ports; leave nil to let the test framework allocate them. |

#### Container image and environment

| Method | Default | Description |
|---|---|---|
| `WithCommitterImage(image string)` | `hyperledger/fabric-x-committer-test-node:1.0.0` | Docker image for the committer container. |
| `WithCommitterEnv(key, value string)` | *(see below)* | Override or add a container environment variable. Pass an empty value to remove a default. |

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
| Endpoints and MSP dirs | *(derived from network)* |

#### Example

```go
fxTopo := nwofabricx.NewDefaultTopology()
fxTopo.AddOrganizationsByName("Org1", "Org2")

// pin a specific committer image
fxTopo.WithCommitterImage("hyperledger/fabric-x-committer-test-node:1.1.0")

// increase block size and turn on debug logging for the orderer component
fxTopo.
    WithCommitterEnv("SC_ORDERER_BLOCK_SIZE", "10").
    WithCommitterEnv("SC_ORDERER_LOGGING_LOGSPEC", "debug")

// remove a default env var entirely
fxTopo.WithCommitterEnv("SC_VC_LOGGING_LOGSPEC", "")

// place the committer in Org2 instead of the default Org1
fxTopo.WithCommitterOrg("Org2")

// rename the sidecar peer (default is "SC")
fxTopo.WithCommitterName("committer")
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
	fxTopo.AddNamespaceWithUnanimity("iou", "Org1")
	
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
