/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
)

func TestEndToEnd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ping Pong Suite")
}

func StartPort() int {
	return integration.PingPongPort.StartPortForNode()
}

func StartPortWithGeneration() int {
	return integration.PingPong2Port.StartPortForNode()
}

func StartPortWithAdmin() int {
	return integration.PingPongWithAdminPort.StartPortForNode()
}
