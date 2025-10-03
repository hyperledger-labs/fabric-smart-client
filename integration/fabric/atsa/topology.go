/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	atsa "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/views"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

func Topology(sdk node.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
	// Create an empty fabric topology
	fabricTopology := fabric.NewDefaultTopology()
	// Enabled Idemix for Anonymous Identities
	fabricTopology.EnableIdemix()
	// Add two organizations with one peer each
	fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")
	// Deploy a dummy chaincode to setup the namespace
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespaceWithUnanimity("asset_transfer", "Org1").SetStateChaincode()

	// Create an empty FSC topology
	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType
	// fscTopology.SetLogging("debug", "")

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

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(sdk)

	return []api.Topology{fabricTopology, fscTopology}
}
