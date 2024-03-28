/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	api2 "github.com/hyperledger-labs/fabric-smart-client/pkg/api"
)

func Topology(sdk api2.SDK, commType fsc.P2PCommunicationType, replicas map[string]int) []api.Topology {
	if replicas == nil {
		replicas = map[string]int{}
	}
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
	approver := fscTopology.AddNodeByName("approver")
	approver.AddOptions(fabric.WithOrganization("Org1"), fsc.WithReplicationFactor(replicas["approver"]))
	approver.RegisterResponder(&views.ApproverView{}, &views.IssueView{})
	approver.RegisterResponder(&views.ApproverView{}, &views.AgreeToSellView{})
	approver.RegisterResponder(&views.ApproverView{}, &views.AgreeToBuyView{})
	approver.RegisterResponder(&views.ApproverView{}, &views.TransferView{})

	// Issuer
	issuer := fscTopology.AddNodeByName("issuer")
	issuer.AddOptions(fabric.WithOrganization("Org3"), fsc.WithReplicationFactor(replicas["issuer"]))
	issuer.RegisterViewFactory("issue", &views.IssueViewFactory{})

	// Alice
	alice := fscTopology.AddNodeByName("alice")
	alice.AddOptions(fabric.WithOrganization("Org2"), fabric.WithAnonymousIdentity(), fsc.WithReplicationFactor(replicas["alice"]))
	alice.RegisterViewFactory("transfer", &views.TransferViewFactory{})
	alice.RegisterViewFactory("agreeToSell", &views.AgreeToSellViewFactory{})
	alice.RegisterViewFactory("agreeToBuy", &views.AgreeToBuyViewFactory{})
	alice.RegisterResponder(&views.AcceptAssetView{}, &views.IssueView{})
	alice.RegisterResponder(&views.TransferResponderView{}, &views.TransferView{})

	// Bob
	bob := fscTopology.AddNodeByName("bob")
	bob.AddOptions(fabric.WithOrganization("Org2"), fabric.WithAnonymousIdentity(), fsc.WithReplicationFactor(replicas["bob"]))
	bob.RegisterViewFactory("transfer", &views.TransferViewFactory{})
	bob.RegisterViewFactory("agreeToSell", &views.AgreeToSellViewFactory{})
	bob.RegisterViewFactory("agreeToBuy", &views.AgreeToBuyViewFactory{})
	bob.RegisterResponder(&views.AcceptAssetView{}, &views.IssueView{})
	bob.RegisterResponder(&views.TransferResponderView{}, &views.TransferView{})

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(sdk)

	return []api.Topology{fabricTopology, fscTopology}
}
