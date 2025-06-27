/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/chaincode/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	api2 "github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

func Topology(sdk api2.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
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
	fscTopology.P2PCommunicationType = commType
	// fscTopology.SetLogging("debug", "")

	// Define Alice's FSC node
	fscTopology.AddNodeByName("alice").
		// Equip it with a Fabric identity from Org1 that is a client
		AddOptions(fabric.WithOrganization("Org1"), fabric.WithClientRole()).
		AddOptions(replicationOpts.For("alice")...).
		// Register the factories of the initiator views for each business process
		RegisterViewFactory("CreateAsset", &views.CreateAssetViewFactory{}).
		RegisterViewFactory("ReadAsset", &views.ReadAssetViewFactory{}).
		RegisterViewFactory("ReadAssetPrivateProperties", &views.ReadAssetPrivatePropertiesViewFactory{}).
		RegisterViewFactory("ChangePublicDescription", &views.ChangePublicDescriptionViewFactory{}).
		RegisterViewFactory("AgreeToSell", &views.AgreeToSellViewFactory{}).
		RegisterViewFactory("AgreeToBuy", &views.AgreeToBuyViewFactory{}).
		RegisterViewFactory("Transfer", &views.TransferViewFactory{})

	// Define Bob's FSC node
	fscTopology.AddNodeByName("bob").
		// Equip it with a Fabric identity from Org2 that is a client
		AddOptions(fabric.WithOrganization("Org2"), fabric.WithClientRole()).
		AddOptions(replicationOpts.For("bob")...).
		// Register the factories of the initiator views for each business process
		RegisterViewFactory("CreateAsset", &views.CreateAssetViewFactory{}).
		RegisterViewFactory("ReadAsset", &views.ReadAssetViewFactory{}).
		RegisterViewFactory("ReadAssetPrivateProperties", &views.ReadAssetPrivatePropertiesViewFactory{}).
		RegisterViewFactory("ChangePublicDescription", &views.ChangePublicDescriptionViewFactory{}).
		RegisterViewFactory("AgreeToSell", &views.AgreeToSellViewFactory{}).
		RegisterViewFactory("AgreeToBuy", &views.AgreeToBuyViewFactory{}).
		RegisterViewFactory("Transfer", &views.TransferViewFactory{})

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(sdk)

	// Done
	return []api.Topology{fabricTopology, fscTopology}
}
