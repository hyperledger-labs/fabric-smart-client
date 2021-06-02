/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package generic

import (
	"path/filepath"
)

func (p *platform) NodeDir(peer *Peer) string {
	return filepath.Join(p.Registry.RootDir, "fscnodes", peer.Name)
}

func (p *platform) NodeConfigPath(peer *Peer) string {
	return filepath.Join(p.NodeDir(peer), "core.yaml")
}
