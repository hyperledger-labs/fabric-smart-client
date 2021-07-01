/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package twonets

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/twonets/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

func Topology() []api.Topology {
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

	// Add alice's FSC node
	alice := fscTopology.AddNodeByName("alice")
	alice.AddOptions(
		fabric.WithNetworkOrganization("alpha", "Org1"),
		fabric.WithNetworkOrganization("beta", "Org3"),
	)
	alice.RegisterViewFactory("ping", &views.PingFactory{})

	// Add bob's FSC node
	bob := fscTopology.AddNodeByName("bob")
	bob.AddOptions(
		fabric.WithNetworkOrganization("alpha", "Org1"),
		fabric.WithNetworkOrganization("beta", "Org3"),
	)
	bob.RegisterResponder(&views.Pong{}, &views.Ping{})

	return []api.Topology{f1Topology, f2Topology, fscTopology}
}
