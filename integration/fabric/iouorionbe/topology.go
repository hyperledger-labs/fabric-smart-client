/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iouorionbe

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/orion"
	api2 "github.com/hyperledger-labs/fabric-smart-client/pkg/api"
)

func Topology(sdk api2.SDK, commType fsc.P2PCommunicationType, replicas map[string]int) []api.Topology {
	if replicas == nil {
		replicas = map[string]int{}
	}
	// Define a Fabric topology with:
	// 1. Three organization: Org1, Org2, and Org3
	// 2. A namespace whose changes can be endorsed by Org1.
	fabricTopology := fabric.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespaceWithUnanimity("iou", "Org1")

	// Define an FSC topology with 3 FCS nodes.
	// One for the approver, one for the borrower, and one for the lender.
	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType
	//fscTopology.SetLogging("debug", "")

	// Add the approver FSC node.
	approver := fscTopology.AddNodeByName("approver")
	// This option equips the approver's FSC node with an identity belonging to Org1.
	// Therefore, the approver is an endorser of the Fabric namespace we defined above.
	approver.AddOptions(
		fabric.WithOrganization("Org1"),
		fabric.WithX509Identity("alice"),
		fsc.WithReplicationFactor(replicas["approver"]),
	)
	approver.RegisterResponder(&views.ApproverView{}, &views.CreateIOUView{})
	approver.RegisterResponder(&views.ApproverView{}, &views.UpdateIOUView{})

	// Add the borrower's FSC node
	borrower := fscTopology.AddNodeByName("borrower")
	borrower.AddOptions(
		fabric.WithOrganization("Org2"),
	)
	borrower.RegisterViewFactory("create", &views.CreateIOUViewFactory{})
	borrower.RegisterViewFactory("update", &views.UpdateIOUViewFactory{})
	borrower.RegisterViewFactory("query", &views.QueryViewFactory{})
	borrowerTopology := orion.SetRemoteDB(borrower)

	// Add the lender's FSC node
	lender := fscTopology.AddNodeByName("lender")
	lender.AddOptions(
		fabric.WithOrganization("Org3"),
		fabric.WithX509Identity("bob"),
		fsc.WithReplicationFactor(replicas["lender"]),
	)
	lender.RegisterResponder(&views.CreateIOUResponderView{}, &views.CreateIOUView{})
	lender.RegisterResponder(&views.UpdateIOUResponderView{}, &views.UpdateIOUView{})
	lender.RegisterViewFactory("query", &views.QueryViewFactory{})

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(sdk)

	return []api.Topology{borrowerTopology, fabricTopology, fscTopology}
}
