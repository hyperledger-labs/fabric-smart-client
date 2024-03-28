/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package twonets

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/twonets/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	api2 "github.com/hyperledger-labs/fabric-smart-client/pkg/api"
)

func Topology(sdk api2.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
	// Define two Fabric topologies
	f1Topology := fabric.NewTopologyWithName("alpha").SetDefault()
	f1Topology.AddOrganizationsByName("Org1", "Org2")
	f1Topology.SetNamespaceApproverOrgs("Org1")
	f1Topology.AddNamespaceWithUnanimity("ns1", "Org1")

	f2Topology := fabric.NewTopologyWithName("beta")
	f2Topology.AddOrganizationsByName("Org3", "Org4")
	f2Topology.SetNamespaceApproverOrgs("Org3")
	f2Topology.AddNamespaceWithUnanimity("ns2", "Org3")

	// Define an FSC topology with 2 FCS nodes.
	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType

	// Add alice's FSC node
	fscTopology.AddNodeByName("alice").
		AddOptions(fabric.WithNetworkOrganization("alpha", "Org1"),
			fabric.WithNetworkOrganization("beta", "Org3"),
			fabric.WithDefaultNetwork("alpha")).
		AddOptions(replicationOpts.For("alice")...).
		RegisterViewFactory("ping", &views.PingFactory{})

	// Add bob's FSC node
	fscTopology.AddNodeByName("bob").
		AddOptions(
			fabric.WithNetworkOrganization("alpha", "Org1"),
			fabric.WithNetworkOrganization("beta", "Org3"),
			fabric.WithDefaultNetwork("beta")).
		AddOptions(replicationOpts.For("bob")...).
		RegisterResponder(&views.Pong{}, &views.Ping{})

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(sdk)

	return []api.Topology{f1Topology, f2Topology, fscTopology}
}
