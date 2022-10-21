# Fabric to Fabric Interoperability view Weaver Relay

[Weaver](https://labs.hyperledger.org/weaver-dlt-interoperability/docs/external/architecture-and-design/overview)
is a platform, a protocol suite, and a set of tools, to enable interoperation for data sharing and asset
movements between independent networks built on heterogeneous blockchain, or more generally, distributed ledger,
technologies, in a manner that preserves the core blockchain tenets of decentralization and security.

In this sample, we will deal with two Fabric networks connected via weaver relays.
Each Fabric network is deployed with a chaincode that implements a simple KVS.
Business parties will be able to interact directly with the `local` chaincode on the Fabric network
they belong to, but also to connect to the `remote` chaincode on the other Fabric network using weaver.

## KVS Chaincode

Let us start by defining the chaincode that will be used on both Fabric networks.

It uses the go contract api to define a simple key-value store. Here is the code:

```go
type SmartContract struct {
contractapi.Contract
}

func (s *SmartContract) Put(ctx contractapi.TransactionContextInterface, key string, value string) error {
return ctx.GetStub().PutState(key, []byte(value))
}

func (s *SmartContract) Get(ctx contractapi.TransactionContextInterface, key string) (string, error) {
v, err := ctx.GetStub().GetState(key)
if err != nil {
return "", errors.Wrapf(err, "failed getting state [%s]", key)
}
err = ctx.GetStub().PutState(key, v)
if err != nil {
return "", errors.Wrapf(err, "failed putting state [%s:%s]", key, string(v))
}
if len(v) == 0 {
return "", nil
}
return string(v), nil
}

func main() {
chaincode, err := contractapi.NewChaincode(new(SmartContract))
if err != nil {
log.Panicf("Error create chaincode: %v", err)
}

if err := chaincode.Start(); err != nil {
log.Panicf("Error starting asset chaincode: %v", err)
}
}
```

## Business Views

To manage the `local` KVS, we will use the following business views:

The following view is used to store or put a key-value pair in the `local` KVS.

```go
type LocalPut struct {
    Chaincode string
    Key       string
    Value     string
}

type LocalPutView struct {
	*LocalPut
}

func (p *LocalPutView) Call(context view.Context) (interface{}, error) {
	// Invoke the passed chaincode to put the key/value pair
	txID, _, err := fabric.GetDefaultChannel(context).Chaincode(p.Chaincode).Invoke(
		"Put", p.Key, p.Value,
	).Call()
	assert.NoError(err, "failed to put key %s", p.Key)

	// return the transaction id
	return txID, nil
}
```

This view is used to retrieve a key pair from the `local` KVS instead.

```go
type LocalGet struct {
	Chaincode string
	Key       string
}

type LocalGetView struct {
	*LocalGet
}

func (g *LocalGetView) Call(context view.Context) (interface{}, error) {
	// Invoke the passed chaincode to get the value corresponding to the passed key
	v, err := fabric.GetDefaultChannel(context).Chaincode(g.Chaincode).Query(
		"Get", g.Key,
	).Call()
	assert.NoError(err, "failed to get key %s", g.Key)

	return v, nil
}
```

To query a key pair from the `remote` KVS, we will use the following business view:

```go
type RemoteGet struct {
	Network   string
	Channel   string
	Chaincode string
	Key       string
}

type RemoteGetView struct {
	*RemoteGet
}

func (g *RemoteGetView) Call(context view.Context) (interface{}, error) {
	// Get a weaver client to the relay of the given network
	relay := weaver.GetProvider(context).Relay(fabric.GetDefaultFNS(context))

	// Build a query to the remote Fabric network.
	// Invoke the `Get` function on the passed key, on the passed chaincode deployed on the passed network and channel.
	query, err := relay.ToFabric().Query(
		fmt.Sprintf("fabric://%s.%s.%s/", g.Network, g.Channel, g.Chaincode),
		"Get", g.Key,
	)
	assert.NoError(err, "failed creating fabric query")

	// Perform the query
	res, err := query.Call()
	assert.NoError(err, "failed querying remote destination")
	assert.NotNil(res, "result should be non-empty")

	// Validate the proof accompanying the result
	proofRaw, err := res.Proof()
	assert.NoError(err, "failed getting proof from query result")
	proof, err := relay.ToFabric().ProofFromBytes(proofRaw)
	assert.NoError(err, "failed unmarshalling proof")
	assert.NoError(proof.Verify(), "failed verifying proof")

	// Inspect the content
	assert.Equal(res.Result(), proof.Result(), "result should be equal, got [%s]!=[%s]", string(res.Result()), string(proof.Result()))
	rwset1, err := res.RWSet()
	assert.NoError(err, "failed getting rwset from result")
	rwset2, err := proof.RWSet()
	assert.NoError(err, "failed getting rwset from proof")
	v1, err := rwset1.GetState(g.Chaincode, g.Key)
	assert.NoError(err, "failed getting key's value from rwset1")
	v2, err := rwset2.GetState(g.Chaincode, g.Key)
	assert.NoError(err, "failed getting key's value from rwset2")
	assert.Equal(v1, v2, "excepted same write [%s]!=[%s]", string(v1), string(v2))

	// return the value of the key, empty if not found
	return v1, nil
}
```

## Testing

To run the sample, one needs first to deploy the `Fabric Smart Client nodes`, the `Fabric networks`, and the `Weaver Relay Network`.
Once these networks are deployed, one can invoke views on the smart client nodes.

So, first step is to describe the topology of the networks we need.

Make sure you have the proper docker images by running `make weaver-docker-image` from the FSC root folder.

### Describe the topology of the networks

To test the above views, we first describe the topology of the networks we need.
Namely, Fabric, FSC, and Weaver Relay networks.

For Fabric, we need two networks. Each network consists of:
- Two organizations, one of which is responsible for the endorsement of the chaincode that implements the KVS;
- One channel, `testchannel`;
- One chaincode implementing the KVS;

For the FSC network, we have a topology with:
1. 2 FCS nodes. One for Alice and one for Bob.
2. Alice is also a member of the first Fabric network. Bob is a member of the second Fabric network.

For the Weaver Relay network, we have a topology with:
- One Weaver Relay node per Fabric network, connected to each other.

We can describe the network topology programmatically as follows:

```go
func Topology() []api.Topology {
// Define two Fabric topologies
f1Topology := fabric.NewTopologyWithName("alpha")
f1Topology.AddOrganizationsByName("Org1", "Org2")
f1Topology.SetNamespaceApproverOrgs("Org1")
f1Topology.AddNamespaceWithUnanimity("ns1", "Org1").SetChaincodePath(
"github.com/hyperledger-labs/fabric-smart-client/samples/fabric/weaver/relay/chaincode",
).NoInit()

f2Topology := fabric.NewTopologyWithName("beta")
f2Topology.EnableGRPCLogging()
f2Topology.AddOrganizationsByName("Org3", "Org4")
f2Topology.SetNamespaceApproverOrgs("Org3")
f2Topology.AddNamespaceWithUnanimity("ns2", "Org3").SetChaincodePath(
"github.com/hyperledger-labs/fabric-smart-client/samples/fabric/weaver/relay/chaincode",
).NoInit()

// Define weaver relay server topology. One relay server per Fabric network
wTopology := weaver.NewTopology()
wTopology.AddRelayServer(f1Topology, "Org1").AddFabricNetwork(f2Topology)
wTopology.AddRelayServer(f2Topology, "Org3").AddFabricNetwork(f1Topology)

// Define an FSC topology with 2 FCS nodes.
fscTopology := fsc.NewTopology()

// Add alice's FSC node
alice := fscTopology.AddNodeByName("alice")
alice.AddOptions(
fabric.WithDefaultNetwork("alpha"),
fabric.WithNetworkOrganization("alpha", "Org1"),
)
alice.RegisterViewFactory("put", &views.LocalPutViewFactory{})
alice.RegisterViewFactory("get", &views.LocalGetViewFactory{})
alice.RegisterViewFactory("remoteGet", &views.RemoteGetViewFactory{})

// Add bob's FSC node
bob := fscTopology.AddNodeByName("bob")
bob.AddOptions(
fabric.WithDefaultNetwork("beta"),
fabric.WithNetworkOrganization("beta", "Org3"),
)
bob.RegisterViewFactory("put", &views.LocalPutViewFactory{})
bob.RegisterViewFactory("get", &views.LocalGetViewFactory{})
bob.RegisterViewFactory("remoteGet", &views.RemoteGetViewFactory{})

return []api.Topology{f1Topology, f2Topology, wTopology, fscTopology}
}
```

### Boostrap the networks

Bootstrap of the networks requires Fabric binaries

To ensure you have the required fabric binary files and set the `FAB_BINS` environment variable to the correct place you can do the following in the project root directory

```shell
make download-fabric
export FAB_BINS=$PWD/../fabric/bin
```

To help us bootstrap the networks and then invoke the business views, the `relay` command line tool is provided.
To build it, we need to run the following command from the folder `$FSC_PATH/samples/fabric/weaver/relay`.
(`$FSC_PATH` refers to the Fabric Smart Client repository in your filesystem see [getting started](../../../../README.md#getting-started))

```shell
go build -o relay
```

If the compilation is successful, we can run the `relay` command line tool as follows:

```shell
./relay network start --path ./testdata
```

The above command will start the networks,
and store all configuration files under the `./testdata` directory.
The CLI will also create the folder `./cmd` that contains a go main file for each FSC node.
The CLI compiles these go main files and then runs them.

If everything is successful, you will see something like the following:

```shell
2022-02-21 15:22:10.855 UTC [nwo.network] Start -> INFO 02b  _____   _   _   ____
2022-02-21 15:22:10.855 UTC [nwo.network] Start -> INFO 02c | ____| | \ | | |  _ \
2022-02-21 15:22:10.855 UTC [nwo.network] Start -> INFO 02d |  _|   |  \| | | | | |
2022-02-21 15:22:10.855 UTC [nwo.network] Start -> INFO 02e | |___  | |\  | | |_| |
2022-02-21 15:22:10.855 UTC [nwo.network] Start -> INFO 02f |_____| |_| \_| |____/
2022-02-21 15:22:10.855 UTC [fsc.integration] Serve -> INFO 030 All GOOD, networks up and running...
2022-02-21 15:22:10.855 UTC [fsc.integration] Serve -> INFO 031 If you want to shut down the networks, press CTRL+C
2022-02-21 15:22:10.856 UTC [fsc.integration] Serve -> INFO 032 Open another terminal to interact with the networks
```

To shut down the networks, just press CTRL-C.

If you want to restart the networks after the shutdown, you can just re-run the above command.
If you don't delete the `./testdata` directory, the network will be started from the previous state.

Before restarting the networks, one can modify the business views to add new functionalities, to fix bugs, and so on.
Upon restarting the networks, the new business views will be available.
Later on, we will see an example of this.

To clean up all artifacts, we can run the following command:

```shell
./relay network clean --path ./testdata
```

The `./testdata` and `./cmd` folders will be deleted.

### Invoke the business views

We start with Alice. Alice stores a key-value pair in her local KVS (the one in the Fabric network Alice has an identity for).

```go
./relay view -c ./testdata/fsc/nodes/alice/client-config.yaml -f put -i "{\"Chaincode\":\"ns1\", \"Key\":\"pineapple\", \"Value\":\"sweet\"}"
```

The above command will output the transaction id of the transaction that Alice has produced.

```shell
"d3dd351f8690654be660ae2f44c0807f3a8017b6c502a79dd2aa8108ff0ef69f"
```

Then, Alice checks the value of her key is actually in her local KVS.

```go
./relay view -c ./testdata/fsc/nodes/alice/client-config.yaml -f get -i "{\"Chaincode\":\"ns1\", \"Key\":\"pineapple\"}"
```

If everything is successful, you will see the value corresponding to the requested key:

```shell
sweet
```

At this point, Bob can query Alice's key. Recall that, Bob does not have a valid identity in the Fabric network where Alice has an identity.
Therefore, Bob cannot call the chaincode directly. Though, Bob can perform that query using Weaver and by invoking the `RemoteGetView` view.

```go
./relay view -c ./testdata/fsc/nodes/bob/client-config.yaml -f remoteGet -i "{\"Network\":\"alpha\",\"Channel\":\"testchannel\",\"Chaincode\":\"ns1\", \"Key\":\"pineapple\"}"
```

Again, if everything is successful, you will see the value corresponding to the requested key:

```shell
sweet
```
