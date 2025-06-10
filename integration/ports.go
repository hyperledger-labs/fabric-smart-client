/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration

import (
	"fmt"
	"os"

	"github.com/onsi/ginkgo/v2"
)

// TestPortRange represents a port range
type TestPortRange int

const (
	basePort      = 5000
	portsPerNode  = 10
	PortsPerSuite = 10 * portsPerNode
)

const (
	BasePort TestPortRange = basePort + PortsPerSuite*iota
	PingPongPort
	PingPong2Port
	PingPongWithAdminPort
	IOUPort
	IOUHSMPort
	AssetTransferSecuredAgreementWithChaincode
	AssetTransferSecuredAgreementWithApprovers
	AssetTransferEventsAgreementWithChaincode
	TwoFabricNetworksPort
	FabricStopRestart
)

// StartPortForNode On linux, the default ephemeral port range is 32768-60999 and can be
// allocated by the system for the client side of TCP connections or when
// programs explicitly request one. Given linux is our default CI system,
// we want to try avoid ports in that range.
func (t TestPortRange) StartPortForNode() int {
	const startEphemeral, endEphemeral = 32768, 60999

	port := int(t) + portsPerNode*(ginkgo.GinkgoParallelProcess()-1)
	if port >= startEphemeral-portsPerNode && port <= endEphemeral-portsPerNode {
		fmt.Fprintf(os.Stderr, "WARNING: port %d is part of the default ephemeral port range on linux. Errors 'address already in use' might occur.", port)
	}
	return port
}
