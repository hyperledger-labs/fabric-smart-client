/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/states"
)

var _ = Describe("EndToEnd", func() {
	var (
		ii *integration.Infrastructure
	)

	AfterEach(func() {
		// Stop the ii
		ii.Stop()
	})

	Describe("Asset Transfer Secured Agreement (With Approvers)", func() {
		var (
			issuer *client.Client
			alice  *client.Client
			bob    *client.Client
		)

		BeforeEach(func() {
			var err error
			// Create the integration ii
			ii, err = integration.Generate(StartPort(), true, fsc.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()

			approver := ii.Identity("approver")

			issuer = client.New(ii.Client("issuer"), ii.Identity("issuer"), approver)
			alice = client.New(ii.Client("alice"), ii.Identity("alice"), approver)
			bob = client.New(ii.Client("bob"), ii.Identity("bob"), approver)
		})

		It("succeeded", func() {
			txID, err := issuer.Issue(&states.Asset{
				ObjectType:        "coin",
				ID:                "1234",
				Owner:             ii.Identity("alice"),
				PublicDescription: "Coin",
				PrivateProperties: []byte("Hello World!!!"),
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(alice.IsTxFinal(txID)).NotTo(HaveOccurred())

			agreementID, err := alice.AgreeToSell(&states.AgreementToSell{
				TradeID: "1234",
				ID:      "1234",
				Price:   100,
			})
			Expect(err).ToNot(HaveOccurred())

			_, err = bob.AgreeToBuy(&states.AgreementToBuy{
				TradeID: "1234",
				ID:      "1234",
				Price:   100,
			})
			Expect(err).ToNot(HaveOccurred())

			err = alice.Transfer("1234", agreementID, ii.Identity("bob"))
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
