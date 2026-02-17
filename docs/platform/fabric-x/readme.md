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
