/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package relay

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/weaver/relay/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/weaver"
	api2 "github.com/hyperledger-labs/fabric-smart-client/pkg/api"
)

func Topology(sdk api2.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
	// Define two Fabric topologies
	f1Topology := fabric.NewTopologyWithName("alpha")
	f1Topology.AddOrganizationsByName("Org1", "Org2")
	f1Topology.SetNamespaceApproverOrgs("Org1")
	f1Topology.AddNamespaceWithUnanimity("ns1", "Org1").SetChaincodePath(
		"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/weaver/relay/chaincode",
	).NoInit()

	f2Topology := fabric.NewTopologyWithName("beta")
	f2Topology.EnableGRPCLogging()
	f2Topology.AddOrganizationsByName("Org3", "Org4")
	f2Topology.SetNamespaceApproverOrgs("Org3")
	f2Topology.AddNamespaceWithUnanimity("ns2", "Org3").SetChaincodePath(
		"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/weaver/relay/chaincode",
	).NoInit()

	// Define weaver relay server topology. One relay server per Fabric network
	wTopology := weaver.NewTopology()
	wTopology.AddRelayServer(f1Topology, "Org1").AddFabricNetwork(f2Topology)
	wTopology.AddRelayServer(f2Topology, "Org3").AddFabricNetwork(f1Topology)

	// Define an FSC topology with 2 FCS nodes.
	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType

	// Add alice's FSC node
	fscTopology.AddNodeByName("alice").
		AddOptions(fabric.WithDefaultNetwork("alpha"), fabric.WithNetworkOrganization("alpha", "Org1")).
		AddOptions(replicationOpts.For("alice")...).
		RegisterViewFactory("init", &views.InitiatorViewFactory{})

	// Add bob's FSC node
	fscTopology.AddNodeByName("bob").
		AddOptions(fabric.WithDefaultNetwork("beta"), fabric.WithNetworkOrganization("beta", "Org3")).
		AddOptions(replicationOpts.For("bob")...).
		RegisterResponder(&views.Responder{}, &views.InitiatorView{})

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(sdk)

	return []api.Topology{f1Topology, f2Topology, wTopology, fscTopology}
}
