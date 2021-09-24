/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

func Topology() []api.Topology {
	// Fabric
	fabricTopology := fabric.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2")
	fabricTopology.AddNamespaceWithUnanimity("asset_transfer", "Org1", "Org2")

	// FSC
	fscTopology := fsc.NewTopology()
	fscTopology.AddNodeByName("alice").AddOptions(
		fabric.WithOrganization("Org1"),
	).RegisterViewFactory("init", &InitiatorViewFactory{})
	fscTopology.AddNodeByName("bob").AddOptions(
		fabric.WithOrganization("Org2"),
	).RegisterResponder(
		&Responder{}, &Initiator{},
	)
	return []api.Topology{fabricTopology, fscTopology}
}
