/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	api2 "github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

func Topology(sdk api2.SDK, commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
	// Fabric
	fabricTopology := fabric.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2")
	fabricTopology.AddNamespaceWithUnanimity("asset_transfer", "Org1", "Org2")

	// FSC
	fscTopology := fsc.NewTopology()
	fscTopology.P2PCommunicationType = commType

	fscTopology.AddNodeByName("alice").
		AddOptions(fabric.WithOrganization("Org1")).
		AddOptions(replicationOpts.For("alice")...).
		RegisterViewFactory("init", &InitiatorViewFactory{})
	fscTopology.AddNodeByName("bob").
		AddOptions(fabric.WithOrganization("Org2")).
		AddOptions(replicationOpts.For("bob")...).
		RegisterResponder(&Responder{}, &Initiator{})

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(sdk)

	return []api.Topology{fabricTopology, fscTopology}
}
