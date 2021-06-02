/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package chaincode

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/chaincode/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

func Topology() []nwo.Topology {
	// Fabric
	fabricTopology := fabric.NewDefaultTopology()
	fabricTopology.EnableNodeOUs()
	fabricTopology.AddOrganizationsByMapping(map[string][]string{
		"Org1": {"org1_peer"},
		"Org2": {"org2_peer"},
	})
	fabricTopology.AddManagedNamespace(
		"asset_transfer",
		`OR ('Org1MSP.member','Org2MSP.member')`,
		"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/chaincode/chaincode",
		"",
		"org1_peer", "org2_peer",
	)

	// FSC
	fscTopology := fsc.NewTopology()

	// Alice
	alice := fscTopology.AddNodeByName("alice")
	alice.AddOptions(fabric.WithOrganization("Org1"), fabric.WithClientRole())
	alice.RegisterViewFactory("CreateAsset", &views.CreateAssetViewFactory{})
	alice.RegisterViewFactory("ReadAsset", &views.ReadAssetViewFactory{})
	alice.RegisterViewFactory("ReadAssetPrivateProperties", &views.ReadAssetPrivatePropertiesViewFactory{})
	alice.RegisterViewFactory("ChangePublicDescription", &views.ChangePublicDescriptionViewFactory{})
	alice.RegisterViewFactory("AgreeToSell", &views.AgreeToSellViewFactory{})
	alice.RegisterViewFactory("AgreeToBuy", &views.AgreeToBuyViewFactory{})
	alice.RegisterViewFactory("Transfer", &views.TransferViewFactory{})

	// Bob
	bob := fscTopology.AddNodeByName("bob")
	bob.AddOptions(fabric.WithOrganization("Org2"), fabric.WithClientRole())
	bob.RegisterViewFactory("CreateAsset", &views.CreateAssetViewFactory{})
	bob.RegisterViewFactory("ReadAsset", &views.ReadAssetViewFactory{})
	bob.RegisterViewFactory("ReadAssetPrivateProperties", &views.ReadAssetPrivatePropertiesViewFactory{})
	bob.RegisterViewFactory("ChangePublicDescription", &views.ChangePublicDescriptionViewFactory{})
	bob.RegisterViewFactory("AgreeToSell", &views.AgreeToSellViewFactory{})
	bob.RegisterViewFactory("AgreeToBuy", &views.AgreeToBuyViewFactory{})
	bob.RegisterViewFactory("Transfer", &views.TransferViewFactory{})

	return []nwo.Topology{fabricTopology, fscTopology}
}
