/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multiendorsement

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
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

func Topology(sdk node.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
	fabricTopology := nwofabricx.NewDefaultTopology()

	// 3 orgs are needed so that the multiendorsement test can update the namespace
	// policy to require endorsements from Org1 and Org2 or Org3.
	fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")

	fabricTopology.SetNamespaceApproverOrgs("Org1")

	// Make the namespace available on the 3 orgs so all orgs can endorse
	// application transactions.
	fabricTopology.AddNamespaceWithUnanimity(Namespace, "Org1", "Org2", "Org3")

	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType
	fscTopology.SetLogging("grpc=error:fabricx=debug:info", "")

	// approver1 keeps the special approver role used by the meta-namespace flow.
	fscTopology.AddNodeByName("approver1").
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(scv2.WithApproverRole()).
		AddOptions(replicationOpts.For("approver1")...).
		RegisterResponder(&simpleviews.ApproveView{}, &simpleviews.CreateView{})

	// approver2 participates in application endorsement
	fscTopology.AddNodeByName("approver2").
		AddOptions(fabric.WithOrganization("Org2")).
		AddOptions(replicationOpts.For("approver2")...).
		RegisterResponder(&simpleviews.ApproveView{}, &simpleviews.CreateView{})

	// approver3 participates in application endorsement
	fscTopology.AddNodeByName("approver3").
		AddOptions(fabric.WithOrganization("Org3")).
		AddOptions(replicationOpts.For("approver3")...).
		RegisterResponder(&simpleviews.ApproveView{}, &simpleviews.CreateView{})

	fscTopology.AddNodeByName(CreatorNode).
		AddOptions(fabric.WithOrganization("Org1")).
		RegisterViewFactory("create", &simpleviews.CreateViewFactory{}).
		RegisterViewFactory("query", &simpleviews.QueryViewFactory{})

	fscTopology.AddSDK(sdk)

	return []api.Topology{
		fabricTopology,
		fscTopology,
	}
}
