/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"fmt"

	fabrictopology "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
)

func ordererTLSServerName(n *network.Network) string {
	if len(n.Orderers) == 0 {
		return ""
	}

	orderer := n.Orderers[0]
	return tlsServerName(orderer, n.Organization(orderer.Organization))
}

func tlsServerName(orderer *fabrictopology.Orderer, org *fabrictopology.Organization) string {
	if orderer == nil || org == nil || org.Domain == "" {
		return ""
	}
	return fmt.Sprintf("%s.%s", orderer.Name, org.Domain)
}
