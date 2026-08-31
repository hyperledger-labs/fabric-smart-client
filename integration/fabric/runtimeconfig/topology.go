/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runtimeconfig

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	iouviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/runtimeconfig/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

type Opts struct {
	CommType        fsc.P2PCommunicationType
	ReplicationOpts *integration.ReplicationOptions
	TLSEnabled      bool
}

// Topology creates the "classic" IOU topology and additionally registers the
// InjectNetworkViewFactory for runtime reconfiguration.
func Topology(opts *Opts) []api.Topology {
	// Define a Fabric topology with:
	// 1. Three organization: Org1, Org2, and Org3
	// 2. A namespace whose changes can be endorsed by Org1.
	// The generated Fabric network configuration and crypto material are not baked into the FSC
	// nodes' core.yaml: each FSC node only gets `fabric:\n  enabled: true` at boot, and the full
	// configuration is injected into every FSC node at runtime by the "inject" view (see
	// commands.InjectAll), demonstrating platform/fabric/core.FSNProvider.AddNetwork.
	fabricTopology := fabric.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespace("iou", topology.Unanimity("Org1"))
	fabricTopology.TLSEnabled = opts.TLSEnabled
	fabricTopology.EnableMinimalFSCFabricConfig()

	// Define an FSC topology with 3 FCS nodes.
	// One for the approver, one for the borrower, and one for the lender.
	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = opts.CommType
	fscTopology.EnablePrometheusMetrics()
	// fscTopology.SetLogging("debug", "")
	fscTopology.EnableTracing(tracing.Otlp)

	// Add the approver FSC node.
	fscTopology.AddNodeByName("approver1").
		// This option equips the approver's FSC node with an identity belonging to Org1.
		// Therefore, the approver is an endorser of the Fabric namespace we defined above.
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(opts.ReplicationOpts.For("approver1")...).
		RegisterResponder(&iouviews.ApproverView{}, &iouviews.CreateIOUView{}).
		RegisterResponder(&iouviews.ApproverView{}, &iouviews.UpdateIOUView{}).
		RegisterViewFactory("init", &iouviews.ApproverInitViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{}).
		RegisterViewFactory("inject", &views.InjectNetworkViewFactory{})

	// Add another approver as well
	fscTopology.AddNodeByName("approver2").
		// This option equips the approver's FSC node with an identity belonging to Org1.
		// Therefore, the approver is an endorser of the Fabric namespace we defined above.
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(opts.ReplicationOpts.For("approver2")...).
		RegisterResponder(&iouviews.ApproverView{}, &iouviews.CreateIOUView{}).
		RegisterResponder(&iouviews.ApproverView{}, &iouviews.UpdateIOUView{}).
		RegisterViewFactory("init", &iouviews.ApproverInitViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{}).
		RegisterViewFactory("inject", &views.InjectNetworkViewFactory{})

	// Add the borrower's FSC node
	fscTopology.AddNodeByName("borrower").
		AddOptions(fabric.WithOrganization("Org2")).
		AddOptions(opts.ReplicationOpts.For("borrower")...).
		RegisterViewFactory("create", &iouviews.CreateIOUViewFactory{}).
		RegisterViewFactory("update", &iouviews.UpdateIOUViewFactory{}).
		RegisterViewFactory("query", &iouviews.QueryViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{}).
		RegisterViewFactory("inject", &views.InjectNetworkViewFactory{})

	// Add the lender's FSC node
	fscTopology.AddNodeByName("lender").
		AddOptions(fabric.WithOrganization("Org3")).
		AddOptions(opts.ReplicationOpts.For("lender")...).
		RegisterResponder(&iouviews.CreateIOUResponderView{}, &iouviews.CreateIOUView{}).
		RegisterResponder(&iouviews.UpdateIOUResponderView{}, &iouviews.UpdateIOUView{}).
		RegisterViewFactory("query", &iouviews.QueryViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{}).
		RegisterViewFactory("inject", &views.InjectNetworkViewFactory{})

	// Monitoring
	monitoringTopology := monitoring.NewTopology()
	monitoringTopology.EnablePrometheusGrafana()
	monitoringTopology.EnableOTLP()

	// Add app-specific SDK to FSC Nodes
	fscTopology.AddSDKForCommType(&SDK{}, opts.CommType)

	return []api.Topology{
		fabricTopology,
		fscTopology,
		monitoringTopology,
	}
}
