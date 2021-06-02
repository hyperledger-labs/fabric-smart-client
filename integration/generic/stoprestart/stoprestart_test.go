/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart_test

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/generic/stoprestart"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("EndToEnd", func() {

	Describe("Stop and Restart", func() {
		var (
			network *integration.Network
		)

		BeforeEach(func() {
			var err error
			// Create the integration network
			network, err = integration.GenNetwork(StartPort(), stoprestart.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration network
			network.Start()
			time.Sleep(3 * time.Second)
		})

		AfterEach(func() {
			// Stop the network
			network.Stop()
		})

		It("stop and restart successfully", func() {
			res, err := network.Client("alice").CallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

			network.StopViewNode("bob")
			time.Sleep(3 * time.Second)
			network.StartViewNode("bob")
			time.Sleep(3 * time.Second)

			res, err = network.Client("alice").CallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})

	})

})
