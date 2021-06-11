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
	// Define a new Fabric topology starting from a Default configuration with a single channel `testchannel`
	// and solo ordering.
	fabricTopology := fabric.NewDefaultTopology()
	// Enabled the NodeOUs capability
	fabricTopology.EnableNodeOUs()
	// Add the organizations, specifying for each organization the names of the peers belonging to that organization
	fabricTopology.AddOrganizationsByMapping(map[string][]string{
		"Org1": {"org1_peer"},
		"Org2": {"org2_peer"},
	})
	// Add a chaincode or `managed namespace`
	fabricTopology.AddManagedNamespace(
		"asset_transfer",
		`OR ('Org1MSP.member','Org2MSP.member')`,
		"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/chaincode/chaincode",
		"",
		"org1_peer", "org2_peer",
	)

	// Define a new FSC topology
	fscTopology := fsc.NewTopology()

	// Define Alice's FSC node
	alice := fscTopology.AddNodeByName("alice")
	// Equip it with a Fabric identity from Org1 that is a client
	alice.AddOptions(fabric.WithOrganization("Org1"), fabric.WithClientRole())
	// Register the factories of the initiator views for each business process
	alice.RegisterViewFactory("CreateAsset", &views.CreateAssetViewFactory{})
	alice.RegisterViewFactory("ReadAsset", &views.ReadAssetViewFactory{})
	alice.RegisterViewFactory("ReadAssetPrivateProperties", &views.ReadAssetPrivatePropertiesViewFactory{})
	alice.RegisterViewFactory("ChangePublicDescription", &views.ChangePublicDescriptionViewFactory{})
	alice.RegisterViewFactory("AgreeToSell", &views.AgreeToSellViewFactory{})
	alice.RegisterViewFactory("AgreeToBuy", &views.AgreeToBuyViewFactory{})
	alice.RegisterViewFactory("Transfer", &views.TransferViewFactory{})

	// Define Bob's FSC node
	bob := fscTopology.AddNodeByName("bob")
	// Equip it with a Fabric identity from Org2 that is a client
	bob.AddOptions(fabric.WithOrganization("Org2"), fabric.WithClientRole())
	// Register the factories of the initiator views for each business process
	bob.RegisterViewFactory("CreateAsset", &views.CreateAssetViewFactory{})
	bob.RegisterViewFactory("ReadAsset", &views.ReadAssetViewFactory{})
	bob.RegisterViewFactory("ReadAssetPrivateProperties", &views.ReadAssetPrivatePropertiesViewFactory{})
	bob.RegisterViewFactory("ChangePublicDescription", &views.ChangePublicDescriptionViewFactory{})
	bob.RegisterViewFactory("AgreeToSell", &views.AgreeToSellViewFactory{})
	bob.RegisterViewFactory("AgreeToBuy", &views.AgreeToBuyViewFactory{})
	bob.RegisterViewFactory("Transfer", &views.TransferViewFactory{})

	// Done
	return []nwo.Topology{fabricTopology, fscTopology}
}
