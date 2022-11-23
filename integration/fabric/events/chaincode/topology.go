/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events/chaincode/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fabric2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
)

func Topology() []api.Topology {
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
	// Define Alice's FSC node
	alice := fscTopology.AddNodeByName("alice")
	// Equip it with a Fabric identity from Org1 that is a client
	alice.AddOptions(fabric.WithOrganization("Org1"), fabric.WithClientRole())
	// Register the factories of the initiator views for each business process
	alice.RegisterViewFactory("EventsView", &views.EventsViewFactory{})
	alice.RegisterViewFactory("MultipleEventsView", &views.MultipleEventsViewFactory{})

	// Define Bob's FSC node
	bob := fscTopology.AddNodeByName("bob")
	// Equip it with a Fabric identity from Org2 that is a client
	bob.AddOptions(fabric.WithOrganization("Org2"), fabric.WithClientRole())
	// Register the factories of the initiator views for each business process
	bob.RegisterViewFactory("EventsView", &views.EventsViewFactory{})

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(&fabric2.SDK{})

	// Done
	return []api.Topology{fabricTopology, fscTopology}
}
