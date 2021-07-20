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
)

func Topology() []api.Topology {
	// Create an empty fabric topology
	fabricTopology := fabric.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2")

	// ERCC_EP="OutOf(2, 'Org1MSP.peer', 'Org2MSP.peer')"
	// ECC_EP="OutOf(2, 'Org1MSP.peer', 'Org2MSP.peer')"
	fabricTopology.AddNamespaceWithUnanimity("echo", "Org1").SetStateChaincode()

	// Create an empty FSC topology
	fscTopology := fsc.NewTopology()

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

	return []api.Topology{fabricTopology, fscTopology}
}
