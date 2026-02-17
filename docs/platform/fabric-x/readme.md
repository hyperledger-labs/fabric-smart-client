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

> TODO: Configuration details for Fabric-x platform here soon.

## Notification Service (Transaction Finality)

The notification service delivers realtime transaction finality events to FSC application nodes. It maintains a persistent bidirectional gRPC stream to the [fabric-x-committer](https://github.com/hyperledger/fabric-x-committer)'s `Notifier` service, which pushes notifications as transactions are committed, invalidated, or timed out. One stream is opened per network/channel pair.

### API

Obtain a `ListenerManager` via `finality.GetListenerManager(sp, network, channel)`, then use:

- `AddFinalityListener(txID, listener)`: registers a `FinalityListener` whose `OnStatus` callback fires once when the transaction reaches finality. Only the first listener for a given txID triggers a subscription request to the committer; subsequent listeners for the same txID piggyback on it. After dispatch, all listeners for the txID are automatically removed.
- `RemoveFinalityListener(txID, listener)`: unregisters a listener before delivery. Safe to call for unknown txIDs or unregistered listeners.

The `OnStatus` callback receives one of: `fdriver.Valid` (committed), `fdriver.Invalid` (rejected), or `fdriver.Unknown` (undetermined / timeout).

### Limitations

- **No automatic reconnection**: if the stream breaks, the manager is removed and registered listeners are lost. A new manager is created on the next `GetListenerManager` call.
- **Handler timeout**: callbacks that exceed 5 seconds (`DefaultHandlerTimeout`) are abandoned with a warning. Handlers that ignore context cancellation will leak a goroutine but won't block other listeners.

### Configuration

```yaml
notificationService:
  endpoints:
    - address: "committer.example.com:9090"
      connectionTimeout: 30s
      tlsEnabled: true
      tlsRootCertFile: "/path/to/ca.pem"
  requestTimeout: 30s
```

### Usage from a View
Refer to the [Simple integration test](/integration/fabricx/simple) for reference.


## Integration Tests

The integration tests for the Fabric-x platform use the [fabric-x-committer-test-node](https://github.com/hyperledger/fabric-x-committer/pkgs/container/fabric-x-committer-test-node) container image.
This container sets up a test networking including:
- An ordering service
- A [committer](https://github.com/hyperledger/fabric-x-committer) instance

The container configuration can be found in: [`integration/nwo/fabricx/extensions/scv2/container.go`](../../../integration/nwo/fabricx/extensions/scv2/container.go).

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