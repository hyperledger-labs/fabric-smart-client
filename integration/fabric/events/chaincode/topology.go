/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events/chaincode/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	api2 "github.com/hyperledger-labs/fabric-smart-client/pkg/api"
)

func Topology(sdk api2.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
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
		"events",
		`OR ('Org1MSP.member','Org2MSP.member')`,
		"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events/chaincode/chaincode",
		"",
		"org1_peer", "org2_peer",
	)

	// Define a new FSC topology
	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType

	// Define Alice's FSC node
	fscTopology.AddNodeByName("alice").
		// Equip it with a Fabric identity from Org1 that is a client
		AddOptions(fabric.WithOrganization("Org1"), fabric.WithClientRole()).
		AddOptions(replicationOpts.For("alice")...).
		// Register the factories of the initiator views for each business process
		RegisterViewFactory("EventsView", &views.EventsViewFactory{}).
		RegisterViewFactory("MultipleEventsView", &views.MultipleEventsViewFactory{}).
		RegisterViewFactory("MultipleListenersView", &views.MultipleListenersViewFactory{})

	// Define Bob's FSC node
	fscTopology.AddNodeByName("bob").
		// Equip it with a Fabric identity from Org2 that is a client
		AddOptions(fabric.WithOrganization("Org2"), fabric.WithClientRole()).
		AddOptions(replicationOpts.For("bob")...).
		// Register the factories of the initiator views for each business process
		RegisterViewFactory("EventsView", &views.EventsViewFactory{})

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(sdk)

	// Done
	return []api.Topology{fabricTopology, fscTopology}
}
