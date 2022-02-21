# Fabric to Fabric Interoperability view Weaver Relay 

Weaver is a platform, a protocol suite, and a set of tools, to enable interoperation for data sharing and asset
movements between independent networks built on heterogeneous blockchain, or more generally, distributed ledger,
technologies, in a manner that preserves the core blockchain tenets of decentralization and security.

https://labs.hyperledger.org/weaver-dlt-interoperability/docs/external/architecture-and-design/overview

In this sample, we will deal with two Fabric networks. In each network, a 

## Testing

To run the sample, one needs first to deploy the `Fabric Smart Client nodes` and the `Fabric networks`.
Once these networks are deployed, one can invoke views on the smart client nodes.

So, first step is to describe the topology of the networks we need.

### Describe the topology of the networks

To test the above views, we have to first clarify the topology of the networks we need.

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

To help us bootstrap the networks and then invoke the business views, the `relay` command line tool is provided.
To build it, we need to run the following command from the folder `$GOPATH/src/github.com/hyperledger-labs/fabric-smart-client/samples/fabric/weaver/relay`.

```shell
go build -o relay
```

If the compilation is successful, we can run the `relay` command line tool as follows:

``` 
./relay network start --path ./testdata
```

The above command will start the Fabric network and the FSC network,
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

```go
./relay view -c ~/testdata/fsc/nodes/alice/client-config.yaml -f put -i "{\"Chaincode\":\"ns1\", \"Key\":\"pineapple\", \"Value\":\"sweet\"}"
```

```go
./relay view -c ~/testdata/fsc/nodes/alice/client-config.yaml -f get -i "{\"Chaincode\":\"ns1\", \"Key\":\"pineapple\"}"
```

```go
./relay view -c ~/testdata/fsc/nodes/bob/client-config.yaml -f remoteGet -i "{\"Network\":\"alpha\",\"Channel\":\"testchannel\",\"Chaincode\":\"ns1\", \"Key\":\"pineapple\"}"
```