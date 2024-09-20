/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	api2 "github.com/hyperledger-labs/fabric-smart-client/pkg/api"
)

func Topology(sdk api2.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
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

	// Approver
	fscTopology.AddNodeByName("approver").
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(replicationOpts.For("approver")...).
		RegisterResponder(&views.ApproverView{}, &views.IssueView{}).
		RegisterResponder(&views.ApproverView{}, &views.AgreeToSellView{}).
		RegisterResponder(&views.ApproverView{}, &views.AgreeToBuyView{}).
		RegisterResponder(&views.ApproverView{}, &views.TransferView{}).
		RegisterViewFactory("finality", &views.FinalityViewFactory{})

	// Issuer
	fscTopology.AddNodeByName("issuer").
		AddOptions(fabric.WithOrganization("Org3")).
		AddOptions(replicationOpts.For("issuer")...).
		RegisterViewFactory("issue", &views.IssueViewFactory{}).
		RegisterViewFactory("finality", &views.FinalityViewFactory{})

	// Alice
	fscTopology.AddNodeByName("alice").
		AddOptions(fabric.WithOrganization("Org2"), fabric.WithAnonymousIdentity()).
		AddOptions(replicationOpts.For("alice")...).
		RegisterViewFactory("transfer", &views.TransferViewFactory{}).
		RegisterViewFactory("agreeToSell", &views.AgreeToSellViewFactory{}).
		RegisterViewFactory("agreeToBuy", &views.AgreeToBuyViewFactory{}).
		RegisterResponder(&views.AcceptAssetView{}, &views.IssueView{}).
		RegisterResponder(&views.TransferResponderView{}, &views.TransferView{}).
		RegisterViewFactory("finality", &views.FinalityViewFactory{})

	// Bob
	fscTopology.AddNodeByName("bob").
		AddOptions(fabric.WithOrganization("Org2"), fabric.WithAnonymousIdentity()).
		AddOptions(replicationOpts.For("bob")...).
		RegisterViewFactory("transfer", &views.TransferViewFactory{}).
		RegisterViewFactory("agreeToSell", &views.AgreeToSellViewFactory{}).
		RegisterViewFactory("agreeToBuy", &views.AgreeToBuyViewFactory{}).
		RegisterResponder(&views.AcceptAssetView{}, &views.IssueView{}).
		RegisterResponder(&views.TransferResponderView{}, &views.TransferView{}).
		RegisterViewFactory("finality", &views.FinalityViewFactory{})

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(sdk)

	return []api.Topology{fabricTopology, fscTopology}
}
