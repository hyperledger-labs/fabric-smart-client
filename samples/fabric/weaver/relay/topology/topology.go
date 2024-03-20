/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

import (
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/weaver"
	fabric2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
	"github.com/hyperledger-labs/fabric-smart-client/samples/fabric/weaver/relay/views"
)

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
	if p2pCommunicationType := os.Getenv("FSC_P2P_COMMUNICATION_TYPE"); len(p2pCommunicationType) > 0 {
		fscTopology.P2PCommunicationType = p2pCommunicationType
	}

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

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(&fabric2.SDK{})

	return []api.Topology{f1Topology, f2Topology, wTopology, fscTopology}
}
