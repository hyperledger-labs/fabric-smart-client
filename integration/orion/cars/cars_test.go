/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cars_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/orion/cars"
	"github.com/hyperledger-labs/fabric-smart-client/integration/orion/cars/views"
)

var _ = Describe("EndToEnd", func() {

	Describe("Car registry demo with Orion Suite", func() {
		var (
			ii *integration.Infrastructure
		)

		BeforeEach(func() {
			var err error
			// Create the integration ii
			ii, err = integration.GenerateAt(StartPort(), "", false, cars.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
			time.Sleep(3 * time.Second)
		})

		AfterEach(func() {
			// Stop the ii
			ii.Stop()
		})

		It("car registry demo", func() {
			_, err := ii.CLI("dealer").CallView("mintRequest", common.JSONMarshall(&views.MintRequest{
				CarRegistration: "RED",
			}))
			Expect(err).NotTo(HaveOccurred())
			_, err = ii.CLI("dealer").CallView("transfer", common.JSONMarshall(&views.Transfer{
				Buyer:           "alice",
				CarRegistration: "RED",
			}))
			Expect(err).NotTo(HaveOccurred())
			_, err = ii.CLI("alice").CallView("transfer", common.JSONMarshall(&views.Transfer{
				Buyer:           "bob",
				CarRegistration: "RED",
			}))
			Expect(err).NotTo(HaveOccurred())
			_, err = ii.CLI("dealer").CallView("mintRequest", common.JSONMarshall(&views.MintRequest{
				CarRegistration: "BLUE",
			}))
			Expect(err).NotTo(HaveOccurred())
			_, err = ii.CLI("dealer").CallView("transfer", common.JSONMarshall(&views.Transfer{
				Buyer:           "bob",
				CarRegistration: "BLUE",
			}))
			Expect(err).NotTo(HaveOccurred())
			_, err = ii.CLI("bob").CallView("transfer", common.JSONMarshall(&views.Transfer{
				Buyer:           "alice",
				CarRegistration: "BLUE",
			}))
			Expect(err).NotTo(HaveOccurred())
		})

	})

})
