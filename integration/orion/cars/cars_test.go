/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cars_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/orion/cars"
	"github.com/hyperledger-labs/fabric-smart-client/integration/orion/cars/views"
)

var _ = Describe("EndToEnd", func() {

	Describe("Ping Pong with Orion Suite", func() {
		var (
			ii *integration.Infrastructure
		)

		BeforeEach(func() {
			var err error
			// Create the integration ii
			ii, err = integration.GenerateAt(StartPort(), "./testdata", cars.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
			time.Sleep(3 * time.Second)
		})

		AfterEach(func() {
			// Stop the ii
			ii.Stop()
		})

		It("ping pong successfully", func() {
			_, err := ii.CLI("dealer").CallView("mintRequest", common.JSONMarshall(&views.MintRequest{
				CarRegistration: "Hello World",
			}))
			Expect(err).NotTo(HaveOccurred())
		})

	})

})
