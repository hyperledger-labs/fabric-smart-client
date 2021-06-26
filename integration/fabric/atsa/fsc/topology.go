/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fsc

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

func Topology() []nwo.Topology {
	// Create an empty fabric topology
	fabricTopology := fabric.NewDefaultTopology()
	// Add two organizations with one peer each
	fabricTopology.AddOrganizationsByName("Org1", "Org2", "Org3")
	// Deploy a dummy chaincode to setup the namespace
	fabricTopology.SetNamespaceApproverOrgs("Org1")
	fabricTopology.AddNamespaceWithUnanimity("asset_transfer", "Org1").SetStateChaincode()

	// Create an empty FSC topology
	fscTopology := fsc.NewTopology()

	// Approver
	approver := fscTopology.AddNodeByName("approver")
	approver.AddOptions(fabric.WithOrganization("Org1"))
	approver.RegisterResponder(&views.ApproverView{}, &views.IssueView{})
	approver.RegisterResponder(&views.ApproverView{}, &views.AgreeToSellView{})
	approver.RegisterResponder(&views.ApproverView{}, &views.AgreeToBuyView{})
	approver.RegisterResponder(&views.ApproverView{}, &views.TransferView{})

	// Issuer
	issuer := fscTopology.AddNodeByName("issuer")
	issuer.AddOptions(fabric.WithOrganization("Org3"))
	issuer.RegisterViewFactory("issue", &views.IssueViewFactory{})

	// Alice
	alice := fscTopology.AddNodeByName("alice")
	alice.AddOptions(fabric.WithOrganization("Org2"))
	alice.RegisterViewFactory("transfer", &views.TransferViewFactory{})
	alice.RegisterViewFactory("agreeToSell", &views.AgreeToSellViewFactory{})
	alice.RegisterViewFactory("agreeToBuy", &views.AgreeToBuyViewFactory{})
	alice.RegisterResponder(&views.AcceptAssetView{}, &views.IssueView{})
	alice.RegisterResponder(&views.TransferResponderView{}, &views.TransferView{})

	// Bob
	bob := fscTopology.AddNodeByName("bob")
	bob.AddOptions(fabric.WithOrganization("Org2"))
	bob.RegisterViewFactory("transfer", &views.TransferViewFactory{})
	bob.RegisterViewFactory("agreeToSell", &views.AgreeToSellViewFactory{})
	bob.RegisterViewFactory("agreeToBuy", &views.AgreeToBuyViewFactory{})
	bob.RegisterResponder(&views.AcceptAssetView{}, &views.IssueView{})
	bob.RegisterResponder(&views.TransferResponderView{}, &views.TransferView{})

	return []nwo.Topology{fabricTopology, fscTopology}
}
