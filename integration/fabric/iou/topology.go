/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring"
	api2 "github.com/hyperledger-labs/fabric-smart-client/pkg/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/tracing"
)

func Topology(sdk api2.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
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
	fscTopology.EnablePrometheusMetrics()

	//fscTopology.SetLogging("debug", "")
	fscTopology.EnableTracing(tracing.Otpl)
	fscTopology.SetLogging("debug", "")

	// Add the approver FSC node.
	fscTopology.AddNodeByName("approver1").
		// This option equips the approver's FSC node with an identity belonging to Org1.
		// Therefore, the approver is an endorser of the Fabric namespace we defined above.
		AddOptions(fabric.WithOrganization("Org1")).
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
		RegisterViewFactory("query", &views.QueryViewFactory{})

	// Monitoring
	monitoringTopology := monitoring.NewTopology()
	monitoringTopology.EnablePrometheusGrafana()
	monitoringTopology.EnableOPTL()

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(sdk)

	return []api.Topology{
		fabricTopology,
		fscTopology,
		monitoringTopology,
	}
}
