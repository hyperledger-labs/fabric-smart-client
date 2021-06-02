/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nochaincode_test

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/nochaincode"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/nochaincode/views"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("EndToEnd", func() {
	var (
		network *integration.Network
	)

	AfterEach(func() {
		// Stop the network
		network.Stop()
	})

	Describe("Asset Transfer Secured Agreement", func() {
		var (
			issuer *nochaincode.Client
			alice  *nochaincode.Client
			bob    *nochaincode.Client
		)

		BeforeEach(func() {
			var err error
			// Create the integration network
			network, err = integration.GenNetwork(StartPort(), nochaincode.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration network
			network.Start()

			approver := network.Identity("approver")

			issuer = nochaincode.NewClient(network.Client("issuer"), network.Identity("issuer"), approver)
			alice = nochaincode.NewClient(network.Client("alice"), network.Identity("alice"), approver)
			bob = nochaincode.NewClient(network.Client("bob"), network.Identity("bob"), approver)
		})

		It("succeeded", func() {
			assetID, err := issuer.Issue(&views.Asset{
				ObjectType:        "coin",
				ID:                "1234",
				Owner:             network.Identity("alice"),
				PublicDescription: "Coin",
				PrivateProperties: []byte("Hello World!!!"),
			})
			Expect(err).ToNot(HaveOccurred())
			time.Sleep(5 * time.Second)

			agreementID, err := alice.AgreeToSell(&views.AgreementToSell{
				TradeID: "1234",
				ID:      "1234",
				Price:   100,
			})
			Expect(err).ToNot(HaveOccurred())
			time.Sleep(5 * time.Second)

			_, err = bob.AgreeToBuy(&views.AgreementToBuy{
				TradeID: "1234",
				ID:      "1234",
				Price:   100,
			})
			Expect(err).ToNot(HaveOccurred())
			time.Sleep(5 * time.Second)

			err = alice.Transfer(assetID, agreementID, network.Identity("bob"))
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
