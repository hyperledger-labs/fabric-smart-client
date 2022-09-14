/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events/chaincode/views"
)

var _ = Describe("EndToEnd", func() {
	var (
		ii *integration.Infrastructure
	)

	AfterEach(func() {
		// Stop the ii
		ii.Stop()
	})

	Describe("Asset Transfer Events (With Chaincode)", func() {
		var (
			alice *chaincode.Client
			// bob   *chaincode.Client
		)

		BeforeEach(func() {
			var err error
			// Create the integration ii
			ii, err = integration.Generate(StartPort(), true, chaincode.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()

			alice = chaincode.NewClient(ii.Client("alice"), ii.Identity("alice"))
			// bob = chaincode.NewClient(ii.Client("bob"), ii.Identity("bob"))
		})

		It("succeeded", func() {
			// Create an asset

			// - Operate from Alice (Org1)
			ap := &views.Asset{
				ID:             "asset1",
				Color:          "blue",
				Size:           35,
				AppraisedValue: 100,
				Owner:          "alice",
			}
			Expect(alice.CreateAsset(ap)).ToNot(HaveOccurred())

		})
	})
})
