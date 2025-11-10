/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package simple

import (
	simpleviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/simple/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/extensions/scv2"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

const (
	ApproverNode = "approver"
	CreatorNode  = "creator"
	Namespace    = "simple"
)

func Topology(sdk node.SDK, commType fsc.P2PCommunicationType) []api.Topology {
	fabricTopology := nwofabricx.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1")
	fabricTopology.AddNamespaceWithUnanimity(Namespace, "Org1")

	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType
	fscTopology.SetLogging("grpc=error:fabricx=debug:info", "")

	fscTopology.AddNodeByName(ApproverNode).
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(scv2.WithApproverRole()).
		// simple approval
		RegisterResponder(&simpleviews.ApproveView{}, &simpleviews.CreateView{})

	fscTopology.AddNodeByName(CreatorNode).
		AddOptions(fabric.WithOrganization("Org1")).
		// simple logic
		RegisterViewFactory("create", &simpleviews.CreateViewFactory{}).
		RegisterViewFactory("query", &simpleviews.QueryViewFactory{})

	fscTopology.AddSDK(sdk)

	return []api.Topology{
		fabricTopology,
		fscTopology,
	}
}
