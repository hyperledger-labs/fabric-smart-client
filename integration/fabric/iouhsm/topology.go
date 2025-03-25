/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iouhsm

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	api2 "github.com/hyperledger-labs/fabric-smart-client/pkg/api"
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

	// Add the approver FSC node.
	fscTopology.AddNodeByName("approver").
		// This option equips the approver's FSC node with an identity belonging to Org1.
		// Therefore, the approver is an endorser of the Fabric namespace we defined above.
		AddOptions(fabric.WithOrganization("Org1"), fabric.WithDefaultIdentityByHSM()).
		AddOptions(replicationOpts.For("approver")...).
		AddSDK(&iou.ApproverSDK{})

	// Add the borrower's FSC node
	fscTopology.AddNodeByName("borrower").
		AddOptions(fabric.WithOrganization("Org2"), fabric.WithDefaultIdentityByHSM(), fabric.WithX509IdentityByHSM("borrower-hsm-2")).
		AddOptions(replicationOpts.For("borrower")...).
		AddSDK(&iou.BorrowerSDK{})

	// Add the lender's FSC node
	fscTopology.AddNodeByName("lender").
		AddOptions(fabric.WithOrganization("Org3"), fabric.WithDefaultIdentityWithLabel("lender")).
		AddOptions(replicationOpts.For("lender")...).
		AddSDK(&iou.LenderSDK{})

	return []api.Topology{fabricTopology, fscTopology}
}
