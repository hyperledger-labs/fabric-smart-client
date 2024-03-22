/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring"
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
	fscTopology.EnableOPTLTracing()

	// Add the approver FSC node.
	approver1 := fscTopology.AddNodeByName("approver1")
	// This option equips the approver's FSC node with an identity belonging to Org1.
	// Therefore, the approver is an endorser of the Fabric namespace we defined above.
	approver1.AddOptions(fabric.WithOrganization("Org1"), fsc.WithReplicationFactor(replicas["approver1"]))
	approver1.RegisterResponder(&views.ApproverView{}, &views.CreateIOUView{})
	approver1.RegisterResponder(&views.ApproverView{}, &views.UpdateIOUView{})
	approver1.RegisterViewFactory("init", &views.ApproverInitViewFactory{})

	// Add another approver as well
	approver2 := fscTopology.AddNodeByName("approver2")
	// This option equips the approver's FSC node with an identity belonging to Org1.
	// Therefore, the approver is an endorser of the Fabric namespace we defined above.
	approver2.AddOptions(fabric.WithOrganization("Org1"), fsc.WithReplicationFactor(replicas["approver2"]))
	approver2.RegisterResponder(&views.ApproverView{}, &views.CreateIOUView{})
	approver2.RegisterResponder(&views.ApproverView{}, &views.UpdateIOUView{})
	approver2.RegisterViewFactory("init", &views.ApproverInitViewFactory{})

	// Add the borrower's FSC node
	borrower := fscTopology.AddNodeByName("borrower")
	borrower.AddOptions(fabric.WithOrganization("Org2"), fsc.WithReplicationFactor(replicas["borrower"]))
	borrower.RegisterViewFactory("create", &views.CreateIOUViewFactory{})
	borrower.RegisterViewFactory("update", &views.UpdateIOUViewFactory{})
	borrower.RegisterViewFactory("query", &views.QueryViewFactory{})

	// Add the lender's FSC node
	lender := fscTopology.AddNodeByName("lender")
	lender.AddOptions(fabric.WithOrganization("Org3"), fsc.WithReplicationFactor(replicas["lender"]))
	lender.RegisterResponder(&views.CreateIOUResponderView{}, &views.CreateIOUView{})
	lender.RegisterResponder(&views.UpdateIOUResponderView{}, &views.UpdateIOUView{})
	lender.RegisterViewFactory("query", &views.QueryViewFactory{})

	// Monitoring
	monitoringTopology := monitoring.NewTopology()
	monitoringTopology.EnableOPTL()

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(sdk)

	return []api.Topology{
		fabricTopology,
		fscTopology,
		monitoringTopology,
	}
}
