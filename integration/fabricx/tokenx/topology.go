/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tokenx

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	tokenviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/tokenx/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/extensions/scv2"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

const (
	// Namespace is the FabricX namespace for token operations
	Namespace = "tokenx"

	// Node names
	ApproverNode = "approver"
	IssuerNode   = "issuer"
	AuditorNode  = "auditor"
)

// Topology creates the network topology for the token management application
// Following the fabricx/simple pattern with separate approver and creator nodes
func Topology(sdk node.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions, owners ...string) []api.Topology {
	// Default owners if none specified
	if len(owners) == 0 {
		owners = []string{"alice", "bob", "charlie"}
	}

	// Create Fabric topology (following simple pattern)
	fabricTopology := nwofabricx.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1")
	fabricTopology.AddNamespaceWithUnanimity(Namespace, "Org1")

	// Create FSC topology
	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType
	fscTopology.SetLogging("grpc=error:fabricx=debug:info", "")

	// Approver node (Org1) - validates and approves transactions
	// This is separate from issuer, following fabricx/simple pattern
	fscTopology.AddNodeByName(ApproverNode).
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(scv2.WithApproverRole()).
		// Approver responders for all transaction types
		RegisterResponder(&tokenviews.ApproverView{}, &tokenviews.IssueView{})

	// Issuer node (Org1) - creates tokens (like creator in simple)
	fscTopology.AddNodeByName(IssuerNode).
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(replicationOpts.For(IssuerNode)...).
		// Issuer views
		RegisterViewFactory("issue", &tokenviews.IssueViewFactory{}).
		RegisterViewFactory("init", &tokenviews.IssuerInitViewFactory{})

	// Owner nodes (Org1) - can hold, transfer, redeem tokens
	for _, ownerName := range owners {
		fscTopology.AddNodeByName(ownerName).
			AddOptions(fabric.WithOrganization("Org1")).
			AddOptions(replicationOpts.For(ownerName)...).
			RegisterViewFactory("query", &tokenviews.BalanceViewFactory{})
	}

	// Add SDK to FSC topology
	fscTopology.AddSDK(sdk)

	return []api.Topology{
		fabricTopology,
		fscTopology,
	}
}
