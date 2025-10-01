/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"fmt"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
)

func peerDockerMSPDir(n *network.Network, p *topology.Peer) string {
	org := n.Organization(p.Organization)

	return filepath.Join(
		"/",
		"root",
		"config",
		"crypto",
		"peerOrganizations",
		org.Domain,
		"peers",
		fmt.Sprintf("%s.%s", p.Name, org.Domain),
		"msp",
	)
}

func rootCrypto(n *network.Network) string {
	return filepath.Join(
		n.RootDir,
		n.Prefix,
		"crypto",
	)
}
