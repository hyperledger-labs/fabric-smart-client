/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	views2 "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/extensions/scv2"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

func Topology(sdk node.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
	// Define a Fabric topology with:
	// 1. Three organization: Org1, Org2, and Org3
	// 2. A namespace whose changes can be endorsed by Org1.
	fabricTopology := nwofabricx.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespaceWithUnanimity("iou", "Org1")

	// Define an FSC topology with 3 FCS nodes.
	// One for the approver, one for the borrower, and one for the lender.
	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType
	fscTopology.SetLogging("grpc=error:fabricx=info:info", "")

	// fscTopology.SetLogging("debug", "")
	// fscTopology.EnableOPTLTracing()

	// Add the approver FSC node.
	fscTopology.AddNodeByName("approver1").
		// This option equips the approver's FSC node with an identity belonging to Org1.
		// Therefore, the approver is an endorser of the Fabric namespace we defined above.
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(scv2.WithApproverRole()).
		AddOptions(replicationOpts.For("approver1")...).
		RegisterResponder(&views.ApproverView{}, &views.CreateIOUView{}).
		RegisterResponder(&views.ApproverView{}, &views.UpdateIOUView{}).
		RegisterViewFactory("init", &views.ApproverInitViewFactory{})

	// Add another approver as well
	fscTopology.AddNodeByName("approver2").
		// This option equips the approver's FSC node with an identity belonging to Org1.
		// Therefore, the approver is an endorser of the Fabric namespace we defined above.
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(replicationOpts.For("approver2")...).
		RegisterResponder(&views.ApproverView{}, &views.CreateIOUView{}).
		RegisterResponder(&views.ApproverView{}, &views.UpdateIOUView{}).
		RegisterViewFactory("init", &views.ApproverInitViewFactory{})

	// Add the borrower's FSC node
	fscTopology.AddNodeByName("borrower").
		AddOptions(fabric.WithOrganization("Org2")).
		AddOptions(replicationOpts.For("borrower")...).
		RegisterViewFactory("create", &views.CreateIOUViewFactory{}).
		RegisterViewFactory("update", &views.UpdateIOUViewFactory{}).
		RegisterViewFactory("query", &views.QueryViewFactory{})

	// Add the lender's FSC node
	fscTopology.AddNodeByName("lender").
		AddOptions(fabric.WithOrganization("Org3")).
		AddOptions(replicationOpts.For("lender")...).
		RegisterResponder(&views.CreateIOUResponderView{}, &views.CreateIOUView{}).
		RegisterResponder(&views.UpdateIOUResponderView{}, &views.UpdateIOUView{}).
		RegisterViewFactory("finality", &views2.FinalityViewFactory{}).
		RegisterViewFactory("query", &views.QueryViewFactory{})

	// Monitoring
	// monitoringTopology := monitoring.NewTopology()
	// monitoringTopology.EnableOPTL()

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(sdk)

	return []api.Topology{
		fabricTopology,
		fscTopology,
		// monitoringTopology,
	}
}
