# Secured Asset Transfer in Fabric

The [Secured asset transfer in Fabric](`https://hyperledger-fabric.readthedocs.io/en/latest/secured_asset_transfer/secured_private_asset_transfer_tutorial.html`) 
is a tutorial that demonstrates how an asset can be represented and traded between organizations 
in a Hyperledger Fabric blockchain channel.

In the next Sections, we will learn:
- The design of the solution used in the sample `Secured asset transfer in Fabric`.
- How to write business views that invoke chaincodes.
- How to write an integration test using the Fabric Smart Client's integration infrastructure.

## Design

### Business Objects

In the `Secured asset transfer in Fabric`, an asset is represented by the following data structure:
```
type Asset struct {   
  ObjectType             string
  ID                     string  
  OwnerOrg               string   
  PublicDescription      string
}
```
An asset:
- Is owned by a single organization (`OwnerOrg` field). State-Based Endorsement is use to model ownership;
- Comes equipped with some private information that is stored in the `implicit collection` of the owning organization;

An asset can be transferred if and only if an agreement between the current owner and the next owner is achieved.
An agreement to sell or buy is represented by the following data structure:
```
type Agreement struct {   
  ID      string   
  Price   int   
  TradeID string
}
```

### Network Topology

For simplicity, let's consider two organizations: `OrgA` and `OrgB`.
Each organization has at least one Fabric peer. 
There are also two clients: Alice who belongs to `OrgA`, and Bob who belongs to `OrgB`.
Each of them run a Fabric Smart Client node equipped with the proper Fabric identity.
Alice's (resp. Bob's) FSC node connects to the Fabric peer in `OrgA` (resp. `OrgB`).

Therefore, we have a vanilla Fabric network with:
- Two Organizations (`OrgA` and `OrgB`);
- Single channel;
- A Chaincode (`asset_transfer`) that coordinates the business operations and can be endorsed by any organization; 
  The chaincode implicitly defines a namespace into the ledger that stores the states.
  The chaincode is available [`here`](./chaincode/asset_transfer.go)
- Support for State-Based Endorsement (SBE, for short) and Implicit collections enabled.

### Business Operations

The chaincode, installed on the peers of all organizations, handles the following business operations:
- `Issue`: Any organization can issue assets because the chaincode endorsement says that any organization can endorse. 
  The issuer passes to the chaincode the asset private information via transient. 
  The chaincode creates a new instance of the asset, filling all fields, and puts it in the RWS. 
  Then, the chaincode sets the SBE policy to assign ownership to the calling organization. 
  Finally, the chaincode stores the asset private information in the implicit collection of the 
  issuer’s organization that is owning the asset.
- `Agree to sell and buy`: Before transferring an Asset, an agreement between the owning organization 
  (`AgreeToSell`) and the recipient organization (`AgreeToBuy`) must be stored on the ledger. 
  Organizations store their agreements in their implicit collections.  The endorsement policy 
  of the implicit collections can be thought as the owner of the agreement.
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
  the asset's owner to new receiving organization and stores the asset in the RWS.
  
## Writing an Integration Test with the Fabric Smart Client

Normally, to run the `Secured asset transfer in Fabric` sample, one would have to deploy the Fabric network, 
invoke the chaincode, and so on, by using a bunch of scripts. 
This is not the most convenient way to test programmatically an application.

FSC provides an `Integration Test Infrastructure` that allow the developer to:
- Describe the topology of the networks used (Fabric and FSC networks);
- Boostrap these networks;
- Initiate interactive protocols to complete given business tasks.

Let us go step by step. 

### Describe the topology of the networks used

This is how the networks can be described programmatically for our `Secured asset transfer in Fabric` sample:

```go
import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/chaincode/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

func Topology() []nwo.Topology {
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
	fabricTopology.AddManagedNamespace(
		"asset_transfer",
		`OR ('Org1MSP.member','Org2MSP.member')`,
		"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/chaincode/chaincode",
		"",
		"org1_peer", "org2_peer",
	)

	// Define a new FSC topology
	fscTopology := fsc.NewTopology()

	// Define Alice's FSC node
	alice := fscTopology.AddNodeByName("alice")
	// Equip it with a Fabric identity from Org1 that is a client
	alice.AddOptions(fabric.WithOrganization("Org1"), fabric.WithClientRole())
	// Register the factories of the initiator views for each business process
	alice.RegisterViewFactory("CreateAsset", &views.CreateAssetViewFactory{})
	alice.RegisterViewFactory("ReadAsset", &views.ReadAssetViewFactory{})
	alice.RegisterViewFactory("ReadAssetPrivateProperties", &views.ReadAssetPrivatePropertiesViewFactory{})
	alice.RegisterViewFactory("ChangePublicDescription", &views.ChangePublicDescriptionViewFactory{})
	alice.RegisterViewFactory("AgreeToSell", &views.AgreeToSellViewFactory{})
	alice.RegisterViewFactory("AgreeToBuy", &views.AgreeToBuyViewFactory{})
	alice.RegisterViewFactory("Transfer", &views.TransferViewFactory{})

	// Define Bob's FSC node
	bob := fscTopology.AddNodeByName("bob")
	// Equip it with a Fabric identity from Org2 that is a client
	bob.AddOptions(fabric.WithOrganization("Org2"), fabric.WithClientRole())
	// Register the factories of the initiator views for each business process
	bob.RegisterViewFactory("CreateAsset", &views.CreateAssetViewFactory{})
	bob.RegisterViewFactory("ReadAsset", &views.ReadAssetViewFactory{})
	bob.RegisterViewFactory("ReadAssetPrivateProperties", &views.ReadAssetPrivatePropertiesViewFactory{})
	bob.RegisterViewFactory("ChangePublicDescription", &views.ChangePublicDescriptionViewFactory{})
	bob.RegisterViewFactory("AgreeToSell", &views.AgreeToSellViewFactory{})
	bob.RegisterViewFactory("AgreeToBuy", &views.AgreeToBuyViewFactory{})
	bob.RegisterViewFactory("Transfer", &views.TransferViewFactory{})

	// Done
	return []nwo.Topology{fabricTopology, fscTopology}
}
```

It is pretty straight-forward the meaning of each step. However, let us dedicate some time to the FSC topology. 
One limitation of golang is that it cannot load code at runtime. This means that all views 
that a node might use must be burned inside the FSC node executable (We will solve this problem 
by adding support for YAEGI, [`Issue 19`](https://github.com/hyperledger-labs/fabric-smart-client/issues/19)).
This is the reason we specify the view factories at topology definition time. At network bootstrapping time,
the Integration Test Infrastructure synthesizes the `main` file for each FSC node and builds it.
The developer does not have to worry about this.

### Boostrap the networks

To bootstrap the networks described by the above topologies, we can simply do this:

```go
    // Create the integration ii
    ii, err := integration.Generate(StartPort(), chaincode.Topology()...)
    Expect(err).NotTo(HaveOccurred())
    // Start the integration ii
    ii.Start()

```
(The above code is taken from this [`test file`](asset_test.go)).

### Initiate a Business Process

In order to exchange an asset, this asset must be created first.
Because the endorsement policy says that any organization can endorse the chaincode,
we can have Alice issuing directly an asset. 
To do so, Alice must contact her FSC node and ask the node to create a new instance of the `CreateAsset` view, on input 
the asset to be created, and run it.
Now, recall that each FSC node exposes a GRPC service, the `View Service`, to do exactly this. Alice just needs to have a client to connect
to this GRPC service and send the proper command. Fortunately enough, the Integration Test Infrastructure 
take care also of this.

To get the View Service client for Alice, we can do the following:
```go
alice := ii.Client("alice")
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
  PublicDescription: ""A new asset for Org1MSP"",
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

func (c *CreateAssetView) Call(context view.Context) (interface{}, error) {
	apRaw, err := c.AssetProperties.Bytes()
	assert.NoError(err, "failed marshalling asset properties struct")

	_, err = context.RunView(
		chaincode.NewInvokeView(
			"asset_transfer",
			"CreateAsset",
			c.AssetProperties.ID,
			c.PublicDescription,
		).WithTransientEntry("asset_properties", apRaw).WithEndorsersFromMyOrg(),
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
inputs. The `NewInvokeView` takes care of the entire endorsement process and blocks until the Fabric reports
back that the transaction has been committed, or a timeout happened.