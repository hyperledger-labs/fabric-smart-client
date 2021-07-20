/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
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
	fabricTopology.AddNamespaceWithUnanimity("echo", "Org1")

	// Create an empty FSC topology
	fscTopology := fsc.NewTopology()

	// Alice
	alice := fscTopology.AddNodeByName("alice")
	alice.AddOptions(fabric.WithOrganization("Org2"))

	// Bob
	bob := fscTopology.AddNodeByName("bob")
	bob.AddOptions(fabric.WithOrganization("Org2"))

	return []api.Topology{fabricTopology, fscTopology}
}
