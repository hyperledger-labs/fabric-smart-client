/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
)

func containerSidecarMSPDir(n *network.Network, p *topology.Peer) string {
	org := n.Organization(p.Organization)

	return path.Join(
		"/",
		"root",
		"artifacts",
		"crypto",
		"peerOrganizations",
		org.Domain,
		"peers",
		fmt.Sprintf("%s.%s", p.Name, org.Domain),
		"msp",
	)
}

func containerSidecarTLSDir(n *network.Network, p *topology.Peer) string {
	org := n.Organization(p.Organization)

	return path.Join(
		"/",
		"root",
		"artifacts",
		"crypto",
		"peerOrganizations",
		org.Domain,
		"peers",
		fmt.Sprintf("%s.%s", p.Name, org.Domain),
		"tls",
	)
}

func containerOrdererTLSDir(n *network.Network, o *topology.Orderer) string {
	org := n.Organization(o.Organization)

	return path.Join(
		"/",
		"root",
		"artifacts",
		"crypto",
		"ordererOrganizations",
		org.Domain,
		"orderers",
		fmt.Sprintf("%s.%s", o.Name, org.Domain),
		"tls",
	)
}

func rootCrypto(n *network.Network) string {
	return filepath.Join(
		n.RootDir,
		n.Prefix,
		"crypto",
	)
}
