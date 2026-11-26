/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package iou is the IOU demo on an EVM network: the topology and the commands its suite runs.
// It lives next to the suite, not in examples/evmiou/iou, because the views import that
// package for its Service.
package evmiou

import (
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/nwo/evm"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

// Topology returns an EVM network whose IOU contract approvers are approver1 and approver2 and
// whose only funded submitter is the borrower, and the four FSC nodes of the demo.
func Topology() []api.Topology {
	evmTopology := evm.NewTopology()
	evmTopology.Approvers = []string{"approver1", "approver2"}
	evmTopology.Submitters = []string{"borrower"}

	fscTopology := fsc.NewTopology()
	for _, name := range []string{"approver1", "approver2"} {
		fscTopology.AddNodeByName(name).
			RegisterResponder(&views.ApproverView{}, &views.CreateIOUView{}).
			RegisterResponder(&views.ApproverView{}, &views.UpdateIOUView{})
	}
	fscTopology.AddNodeByName("borrower").
		RegisterViewFactory("create", &views.CreateIOUViewFactory{}).
		RegisterViewFactory("update", &views.UpdateIOUViewFactory{}).
		RegisterViewFactory("query", &views.QueryViewFactory{})
	fscTopology.AddNodeByName("lender").
		RegisterResponder(&views.CreateIOUResponderView{}, &views.CreateIOUView{}).
		RegisterResponder(&views.UpdateIOUResponderView{}, &views.UpdateIOUView{}).
		RegisterViewFactory("query", &views.QueryViewFactory{})
	fscTopology.AddSDKWithExtensions(&iou.SDK{})

	return []api.Topology{evmTopology, fscTopology}
}
