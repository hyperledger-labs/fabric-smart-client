# Secured asset transfer in Fabric

This tutorial teaches how to drive a Fabric chaincode from FSC views and test it with the integration infrastructure. It belongs to the [Fabric platform](../../../docs/platform/fabric/README.md) guide.

The [Secured asset transfer in Fabric](https://hyperledger-fabric.readthedocs.io/en/latest/secured_asset_transfer/secured_private_asset_transfer_tutorial.html)
is a tutorial that demonstrates how an asset can be represented and traded between organizations
in a Hyperledger Fabric blockchain channel.

In the next sections, we will learn:

- The design of the solution used in the sample `Secured asset transfer in Fabric`.
- How to write business views that invoke chaincodes.
- How to write an integration test using the Fabric Smart Client's integration infrastructure.

## Design

### Business objects

In the `Secured asset transfer in Fabric`, an asset is represented by the following data structure:

```go
type Asset struct {
  ObjectType             string
  ID                     string
  OwnerOrg               string
  PublicDescription      string
}
```

An asset:

- Is owned by a single organization (`OwnerOrg` field). State-Based Endorsement is used to model ownership;
- Comes equipped with some private information that is stored in the `implicit collection` of the owning organization;

An asset can be transferred if and only if an agreement between the current owner and the next owner is achieved.
An agreement to sell or buy is represented by the following data structure:

```go
type Agreement struct {
  ID      string
  Price   int
  TradeID string
}
```

### Network topology

For simplicity, let's consider two organizations: `Org1` and `Org2`.
Each organization has at least one Fabric peer.
There are also two clients: Alice who belongs to `Org1`, and Bob who belongs to `Org2`.
Each of them runs a Fabric Smart Client node equipped with the proper Fabric identity.
Alice's (resp. Bob's) FSC node connects to the Fabric peer in `Org1` (resp. `Org2`).

Therefore, we have a vanilla Fabric network with:

- Two Organizations (`Org1` and `Org2`);
- Single channel;
- A Chaincode (`asset_transfer`) that coordinates the business operations and can be endorsed by any organization;
  The chaincode implicitly defines a namespace into the ledger that stores the states.
  The chaincode is available [`here`](chaincode/asset_transfer.go)
- Support for State-Based Endorsement (SBE, for short) and Implicit collections enabled.

### Business operations

The chaincode, installed on the peers of all organizations, handles the following business operations:

- `Issue`: Any organization can issue assets because the chaincode endorsement says that any organization can endorse.
  The issuer passes to the chaincode the asset private information via transient.
  The chaincode creates a new instance of the asset, filling all fields, and puts it in the RWS.
  Then, the chaincode sets the SBE policy to assign ownership to the calling organization.
  Finally, the chaincode stores the asset private information in the implicit collection of the
  issuer’s organization that is owning the asset.
- `Agree to sell and buy`: Before transferring an Asset, an agreement between the owning organization
  (`AgreeToSell`) and the recipient organization (`AgreeToBuy`) must be stored on the ledger.
  Organizations store their agreements in their implicit collections. The endorsement policy
  of the implicit collections can be thought of as the owner of the agreement.
  A `Hash` appears on the ledger for each agreement and this hash is available to the entire network.
  A transfer can happen only if there is a matching agreement to sell and buy.
  The chaincode performs access control and then stores the agreement on the implicit collection
  of the calling organization.
- `Transfer`: The asset owner passes to the chaincode the asset private information via transient.
  The chaincode first checks that the asset private information is compatible with the hash stored in owner’s
  organization implicit collection. If there is a match between the `AgreeToSell` and the `AgreeToBuy`
  (The match is performed by comparing the hashes. Notice that hashes are available to all peers in the network),
  the chaincode writes the private information in the next owner’s organization implicit collection
  (This requires an endorsement from the next owner’s organization). Finally, the chaincode changes
  the asset's owner to the new receiving organization and stores the asset in the RWS.

## Writing an integration test with the Fabric Smart Client

Normally, to run the `Secured asset transfer in Fabric` sample, one would have to deploy the Fabric network,
invoke the chaincode, and so on, by using a bunch of scripts.
This is not the most convenient way to test programmatically an application.

FSC provides an `Integration Test Infrastructure` that allows the developer to:

- Describe the topology of the networks used (Fabric and FSC networks);
- Bootstrap these networks;
- Initiate interactive protocols to complete given business tasks.

Let us go step by step.

### Describe the topology of the networks used

This is how the networks can be described programmatically for our `Secured asset transfer in Fabric` sample:

```go
import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsachaincode/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fabricsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
)

func Topology(replicationOpts *integration.ReplicationOptions) []api.Topology {
	// Define a new Fabric topology starting from a Default configuration with a single channel `testchannel`
	// and solo ordering.
	fabricTopology := fabric.NewDefaultTopology()
	// Enabled the NodeOUs capability
	fabricTopology.EnableNodeOUs()
	// Add the organizations, specifying for each organization the names of the peers belonging to that organization
	fabricTopology.AddOrganizationsByMapping(map[string][]string{
		"Org1": {"org1_peer"},
		"Org2": {"org2_peer"},
	})
	// Add a chaincode or `managed namespace`
	fabricTopology.AddNamespace("asset_transfer",
		topology.Signature(`OR ('Org1MSP.member','Org2MSP.member')`),
		topology.WithContainerImage("fsc-cc/atsachaincode:latest"),
		topology.WithPeers("org1_peer", "org2_peer"),
	)

	// Define a new FSC topology
	fscTopology := fsc.NewTopology()

	// Define Alice's FSC node
	fscTopology.AddNodeByName("alice").
		// Equip it with a Fabric identity from Org1 that is a client
		AddOptions(fabric.WithOrganization("Org1"), fabric.WithClientRole()).
		AddOptions(replicationOpts.For("alice")...).
		// Register the factories of the initiator views for each business process
		RegisterViewFactory("CreateAsset", &views.CreateAssetViewFactory{}).
		RegisterViewFactory("ReadAsset", &views.ReadAssetViewFactory{}).
		RegisterViewFactory("ReadAssetPrivateProperties", &views.ReadAssetPrivatePropertiesViewFactory{}).
		RegisterViewFactory("ChangePublicDescription", &views.ChangePublicDescriptionViewFactory{}).
		RegisterViewFactory("AgreeToSell", &views.AgreeToSellViewFactory{}).
		RegisterViewFactory("AgreeToBuy", &views.AgreeToBuyViewFactory{}).
		RegisterViewFactory("Transfer", &views.TransferViewFactory{})

	// Define Bob's FSC node
	fscTopology.AddNodeByName("bob").
		// Equip it with a Fabric identity from Org2 that is a client
		AddOptions(fabric.WithOrganization("Org2"), fabric.WithClientRole()).
		AddOptions(replicationOpts.For("bob")...).
		// Register the factories of the initiator views for each business process
		RegisterViewFactory("CreateAsset", &views.CreateAssetViewFactory{}).
		RegisterViewFactory("ReadAsset", &views.ReadAssetViewFactory{}).
		RegisterViewFactory("ReadAssetPrivateProperties", &views.ReadAssetPrivatePropertiesViewFactory{}).
		RegisterViewFactory("ChangePublicDescription", &views.ChangePublicDescriptionViewFactory{}).
		RegisterViewFactory("AgreeToSell", &views.AgreeToSellViewFactory{}).
		RegisterViewFactory("AgreeToBuy", &views.AgreeToBuyViewFactory{}).
		RegisterViewFactory("Transfer", &views.TransferViewFactory{})

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(&fabricsdk.SDK{})

	// Done
	return []api.Topology{fabricTopology, fscTopology}
}
```

It is pretty straight-forward the meaning of each step. However, let us dedicate some time to the FSC topology.
One limitation of golang is that it cannot load code at runtime. This means that all views
that a node might use must be burned inside the FSC node executable.
This is the reason we specify the view factories at topology definition time. At network bootstrapping time,
the Integration Test Infrastructure synthesizes the `main` file for each FSC node and builds it.
The developer does not have to worry about this.

### Bootstrap the networks

To bootstrap the networks described by the above topologies, we do this:

```go
s := &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
	return integration.GenerateAt(StartPort(), "out/testdata", integration.WithRaceDetection, atsa.Topology(integration.NoReplication)...)
})}
// Create and start the networks
s.Setup()
```

(The above code is adapted from this [`test file`](atsa_test.go), which imports this package as `atsa` and calls `s.Setup` from a
`BeforeEach`. `StartPort` is defined in the suite's [`atsa_suite_test.go`](atsa_suite_test.go).)

### Initiate a business process

In order to exchange an asset, this asset must be created first.
Because the endorsement policy says that any organization can endorse the chaincode,
we can have Alice issuing directly an asset.
To do so, Alice must contact her FSC node and ask the node to create a new instance of the `CreateAsset` view, on input
the asset to be created, and run it.
Now, recall that each FSC node exposes a gRPC service, the `View Service`, to do exactly this. Alice just needs to have a client to connect
to this gRPC service and send the proper command. The Integration Test Infrastructure
takes care of this.

To get the View Service client for Alice, we can do the following:

```go
alice := s.II.Client("alice")
```

At this point, we are ready to invoke the `CreateAsset` view, like this:

```go
ap := &views.AssetProperties{
  ObjectType: "asset_properties",
  ID:         "asset1",
  Color:      "blue",
  Size:       35,
  Salt:       nonce,
}

_, err := alice.CallView("CreateAsset", common.JSONMarshall(&views.CreateAsset{
  AssetProperties:   ap,
  PublicDescription: "A new asset for Org1MSP",
}))
Expect(err).NotTo(HaveOccurred())
```

It is as simple as that.

But, what does the `CreateAsset` view do exactly?
Here is the definition of the `CreateAsset` view and its factory:

```go
type CreateAsset struct {
	AssetProperties   *AssetProperties
	PublicDescription string
}

type CreateAssetView struct {
	*CreateAsset
}

func (c *CreateAssetView) Call(viewCtx view.Context) (any, error) {
	apRaw, err := c.AssetProperties.Bytes()
	assert.NoError(err, "failed marshalling asset properties struct")

	_, err = viewCtx.RunView(
		chaincode.NewInvokeView(
			"asset_transfer",
			"CreateAsset",
			c.AssetProperties.ID,
			c.PublicDescription,
		).WithTransientEntry("asset_properties", apRaw).WithEndorsersFromMyOrg().WithNumRetries(2).WithRetrySleep(2 * time.Second),
	)
	assert.NoError(err, "failed creating asset")

	return nil, nil
}

type CreateAssetViewFactory struct{}

func (c *CreateAssetViewFactory) NewView(in []byte) (view.View, error) {
	f := &CreateAssetView{CreateAsset: &CreateAsset{}}
	err := json.Unmarshal(in, f.CreateAsset)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}
```

This view is very simple, it just invokes the `CreateAsset` function of the `asset_transfer` chaincode passing the proper
inputs. The `NewInvokeView` takes care of the entire endorsement process and blocks until Fabric reports
back that the transaction has been committed, or a timeout happened.
