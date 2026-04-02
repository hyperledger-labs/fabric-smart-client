/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deployment

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	simpleviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/simple/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

const (
	ApproverNode = "approver"
	CreatorNode  = "creator"
	Namespace    = "simple"
)

func Topology(sdk node.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
	fabricTopology := nwofabricx.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespaceWithUnanimity(Namespace, "Org1")

	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType
	fscTopology.SetLogging("grpc=error:fabricx=debug:info", "")

	// Add the approver FSC node.
	fscTopology.AddNodeByName("approver1").
		// This option equips the approver's FSC node with an identity belonging to Org1.
		// Therefore, the approver is an endorser of the Fabric namespace we defined above.
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(replicationOpts.For("approver1")...).
		RegisterResponder(&simpleviews.ApproveView{}, &simpleviews.CreateView{})

	// Add another approver as well
	fscTopology.AddNodeByName("approver2").
		// This option equips the approver's FSC node with an identity belonging to Org1.
		// Therefore, the approver is an endorser of the Fabric namespace we defined above.
		AddOptions(fabric.WithOrganization("Org2")).
		AddOptions(replicationOpts.For("approver2")...).
		RegisterResponder(&simpleviews.ApproveView{}, &simpleviews.CreateView{})

	fscTopology.AddNodeByName(CreatorNode).
		AddOptions(fabric.WithOrganization("Org3")).
		// simple logic
		RegisterViewFactory("create", &simpleviews.CreateViewFactory{}).
		RegisterViewFactory("query", &simpleviews.QueryViewFactory{})

	fscTopology.AddSDK(sdk)

	return []api.Topology{
		fabricTopology,
		fscTopology,
	}
}
