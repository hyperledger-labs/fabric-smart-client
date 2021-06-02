/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package network

import (
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

func (n *Network) NodeDir(peer *topology.Peer) string {
	return filepath.Join(n.Registry.RootDir, "fscnodes", peer.Name)
}

func (n *Network) NodeConfigPath(peer *topology.Peer) string {
	return filepath.Join(n.NodeDir(peer), "core.yaml")
}
