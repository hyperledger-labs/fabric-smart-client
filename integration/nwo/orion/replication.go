/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
)

// SetFSCBackend creates a new FSC node that is replicated to the given other nodes.
func SetFSCBackend(node *node.Node) *Topology {
	orionTopologyName := node.Name + "-orion-backend"
	orionTopology := NewTopology()
	orionTopology.SetName(orionTopologyName)
	orionTopology.AddDB("backend", node.Name)
	node.AddOptions(
		fsc.WithOrionPersistence(orionTopologyName, "backend", node.Name),
		fabric.WithOrionVaultPersistence(orionTopologyName, "backend", node.Name),
		WithRole(node.Name),
	)
	orionTopology.SetDefaultSDKOnNodes(node)
	return orionTopology
}
