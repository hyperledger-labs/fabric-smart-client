/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package atsa

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	atsa "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/views"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

func Topology(sdk node.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
	// Create an fabric-x topology with idemix enabled
	fabricTopology := nwofabricx.NewDefaultTopology()
	fabricTopology.EnableIdemix()
	fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespaceWithUnanimity("asset_transfer", "Org1")

	// Create an FSC topology
	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType
	fscTopology.SetLogging("grpc=error:fabricx=debug:debug", "")

	// Approver
	fscTopology.AddNodeByName("approver").
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(replicationOpts.For("approver")...).
		RegisterResponder(&atsa.ApproverView{}, &atsa.IssueView{}).
		RegisterResponder(&atsa.ApproverView{}, &atsa.AgreeToSellView{}).
		RegisterResponder(&atsa.ApproverView{}, &atsa.AgreeToBuyView{}).
		RegisterResponder(&atsa.ApproverView{}, &atsa.TransferView{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{})

	// Issuer
	fscTopology.AddNodeByName("issuer").
		AddOptions(fabric.WithOrganization("Org3")).
		AddOptions(replicationOpts.For("issuer")...).
		RegisterViewFactory("issue", &atsa.IssueViewFactory{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{})

	// Alice
	fscTopology.AddNodeByName("alice").
		AddOptions(fabric.WithOrganization("Org2"), fabric.WithAnonymousIdentity()).
		AddOptions(replicationOpts.For("alice")...).
		RegisterViewFactory("transfer", &atsa.TransferViewFactory{}).
		RegisterViewFactory("agreeToSell", &atsa.AgreeToSellViewFactory{}).
		RegisterViewFactory("agreeToBuy", &atsa.AgreeToBuyViewFactory{}).
		RegisterResponder(&atsa.AcceptAssetView{}, &atsa.IssueView{}).
		RegisterResponder(&atsa.TransferResponderView{}, &atsa.TransferView{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{})

	// Bob
	fscTopology.AddNodeByName("bob").
		AddOptions(fabric.WithOrganization("Org2"), fabric.WithAnonymousIdentity()).
		AddOptions(replicationOpts.For("bob")...).
		RegisterViewFactory("transfer", &atsa.TransferViewFactory{}).
		RegisterViewFactory("agreeToSell", &atsa.AgreeToSellViewFactory{}).
		RegisterViewFactory("agreeToBuy", &atsa.AgreeToBuyViewFactory{}).
		RegisterResponder(&atsa.AcceptAssetView{}, &atsa.IssueView{}).
		RegisterResponder(&atsa.TransferResponderView{}, &atsa.TransferView{}).
		RegisterViewFactory("finality", &cviews.FinalityViewFactory{})

	// Add app-specific SDK to FSC Nodes
	fscTopology.AddSDK(sdk)

	return []api.Topology{fabricTopology, fscTopology}
}
