/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring"
	api2 "github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/tracing"
)

type Opts struct {
	SDK             api2.SDK
	CommType        fsc.P2PCommunicationType
	ReplicationOpts *integration.ReplicationOptions
	TLSEnabled      bool
}

func Topology(opts *Opts) []api.Topology {
	// Define a Fabric topology with:
	// 1. Three organization: Org1, Org2, and Org3
	// 2. A namespace whose changes can be endorsed by Org1.
	fabricTopology := fabric.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespaceWithUnanimity("iou", "Org1")
	fabricTopology.TLSEnabled = opts.TLSEnabled

	// Define an FSC topology with 3 FCS nodes.
	// One for the approver, one for the borrower, and one for the lender.
	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = opts.CommType
	fscTopology.EnablePrometheusMetrics()
	// fscTopology.SetLogging("debug", "")

	// fscTopology.SetLogging("debug", "")
	fscTopology.EnableTracing(tracing.Otpl)

	// Add the approver FSC node.
	fscTopology.AddNodeByName("approver1").
		// This option equips the approver's FSC node with an identity belonging to Org1.
		// Therefore, the approver is an endorser of the Fabric namespace we defined above.
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(opts.ReplicationOpts.For("approver1")...).
		RegisterResponder(&views.ApproverView{}, &views.CreateIOUView{}).
		RegisterResponder(&views.ApproverView{}, &views.UpdateIOUView{}).
		RegisterViewFactory("init", &views.ApproverInitViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{})

	// Add another approver as well
	fscTopology.AddNodeByName("approver2").
		// This option equips the approver's FSC node with an identity belonging to Org1.
		// Therefore, the approver is an endorser of the Fabric namespace we defined above.
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(opts.ReplicationOpts.For("approver2")...).
		RegisterResponder(&views.ApproverView{}, &views.CreateIOUView{}).
		RegisterResponder(&views.ApproverView{}, &views.UpdateIOUView{}).
		RegisterViewFactory("init", &views.ApproverInitViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{})

	// Add the borrower's FSC node
	fscTopology.AddNodeByName("borrower").
		AddOptions(fabric.WithOrganization("Org2")).
		AddOptions(opts.ReplicationOpts.For("borrower")...).
		RegisterViewFactory("create", &views.CreateIOUViewFactory{}).
		RegisterViewFactory("update", &views.UpdateIOUViewFactory{}).
		RegisterViewFactory("query", &views.QueryViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{})

	// Add the lender's FSC node
	fscTopology.AddNodeByName("lender").
		AddOptions(fabric.WithOrganization("Org3")).
		AddOptions(opts.ReplicationOpts.For("lender")...).
		RegisterResponder(&views.CreateIOUResponderView{}, &views.CreateIOUView{}).
		RegisterResponder(&views.UpdateIOUResponderView{}, &views.UpdateIOUView{}).
		RegisterViewFactory("query", &views.QueryViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{})

	// Monitoring
	monitoringTopology := monitoring.NewTopology()
	monitoringTopology.EnablePrometheusGrafana()
	monitoringTopology.EnableOPTL()

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(opts.SDK)

	return []api.Topology{
		fabricTopology,
		fscTopology,
		monitoringTopology,
	}
}
