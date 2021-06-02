/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong_test

import (
	"encoding/json"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"

	"github.com/hyperledger-labs/fabric-smart-client/integration"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestEndToEnd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ping Pong Suite (on Generic Platform")
}

var (
	buildServer *network.BuildServer
	components  *common.Components
)

var _ = SynchronizedBeforeSuite(func() []byte {
	buildServer = network.NewBuildServer()
	buildServer.Serve()

	components = buildServer.Components()
	payload, err := json.Marshal(components)
	Expect(err).NotTo(HaveOccurred())

	return payload
}, func(payload []byte) {
	err := json.Unmarshal(payload, &components)
	Expect(err).NotTo(HaveOccurred())
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	buildServer.Shutdown()
})

func StartPort() int {
	return integration.PingPongPort.StartPortForNode()
}

func StartPort2() int {
	return integration.PingPong2Port.StartPortForNode()
}
