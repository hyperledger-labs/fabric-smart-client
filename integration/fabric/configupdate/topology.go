/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configupdate

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/configupdate/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou"
	iouviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

type Opts struct {
	CommType        fsc.P2PCommunicationType
	ReplicationOpts *integration.ReplicationOptions
	TLSEnabled      bool
}

// Topology is the IOU topology, reduced to what a channel-configuration test needs.
//
// It reuses the IOU views and SDK verbatim: the flow itself is not what is under test,
// it is the load-bearing assertion. If the FSC nodes mishandle the CONFIG block that
// arrives mid-test — losing their view of the channel's MSPs, or dropping the ordering
// endpoints that platform/fabric/core/generic/committer.applyConfigUpdates re-reads —
// the next IOU transaction cannot be endorsed, ordered or finalised, and fails.
//
// Monitoring and tracing are deliberately left out: they add container startup cost and
// nothing here asserts on them.
func Topology(opts *Opts) []api.Topology {
	// A Fabric topology with three organizations and a namespace endorsed by Org1,
	// as in the IOU test. The channel starts with the configtx defaults, notably a
	// BatchTimeout of 1s, which the test then changes at runtime.
	fabricTopology := fabric.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespace("iou", topology.Unanimity("Org1"))
	fabricTopology.TLSEnabled = opts.TLSEnabled

	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = opts.CommType

	// Two approvers, both in Org1, so that the flow before and after a config update can
	// be driven through different endorsers.
	for _, approver := range []string{"approver1", "approver2"} {
		fscTopology.AddNodeByName(approver).
			AddOptions(fabric.WithOrganization("Org1")).
			AddOptions(opts.ReplicationOpts.For(approver)...).
			RegisterResponder(&iouviews.ApproverView{}, &iouviews.CreateIOUView{}).
			RegisterResponder(&iouviews.ApproverView{}, &iouviews.UpdateIOUView{}).
			RegisterViewFactory("init", &iouviews.ApproverInitViewFactory{}).
			RegisterViewFactory("finality", &cviews.FinalityViewFactory{}).
			RegisterViewFactory("configseq", &views.ConfigSequenceViewFactory{})
	}

	fscTopology.AddNodeByName("borrower").
		AddOptions(fabric.WithOrganization("Org2")).
		AddOptions(opts.ReplicationOpts.For("borrower")...).
		RegisterViewFactory("create", &iouviews.CreateIOUViewFactory{}).
		RegisterViewFactory("update", &iouviews.UpdateIOUViewFactory{}).
		RegisterViewFactory("query", &iouviews.QueryViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{}).
		RegisterViewFactory("configseq", &views.ConfigSequenceViewFactory{})

	fscTopology.AddNodeByName("lender").
		AddOptions(fabric.WithOrganization("Org3")).
		AddOptions(opts.ReplicationOpts.For("lender")...).
		RegisterResponder(&iouviews.CreateIOUResponderView{}, &iouviews.CreateIOUView{}).
		RegisterResponder(&iouviews.UpdateIOUResponderView{}, &iouviews.UpdateIOUView{}).
		RegisterViewFactory("query", &iouviews.QueryViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{}).
		RegisterViewFactory("configseq", &views.ConfigSequenceViewFactory{})

	fscTopology.AddSDKForCommType(&iou.SDK{}, opts.CommType)

	return []api.Topology{
		fabricTopology,
		fscTopology,
	}
}
