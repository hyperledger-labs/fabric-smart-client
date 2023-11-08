/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events/chaincode/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EndToEnd", func() {
	var (
		ii *integration.Infrastructure
	)

	AfterEach(func() {
		// Stop the ii
		ii.Stop()
	})

	Describe("Events (With Chaincode)", func() {
		var (
			alice *chaincode.Client
			bob   *chaincode.Client
		)

		BeforeEach(func() {
			var err error
			ii, err = integration.Generate(StartPort(), true, chaincode.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()

			alice = chaincode.NewClient(ii.Client("alice"), ii.Identity("alice"))
			bob = chaincode.NewClient(ii.Client("bob"), ii.Identity("bob"))
		})

		It("clients listening to single chaincode events", func() {
			// - Operate from Alice (Org1)

			event, err := alice.EventsView("CreateAsset", "CreateAsset")

			Expect(err).ToNot(HaveOccurred())
			eventReceived := &views.EventReceived{}
			json.Unmarshal(event.([]byte), eventReceived)
			Expect(string(eventReceived.Event.Payload)).To(Equal("Invoked Create Asset Successfully"))

			// - Operate from Bob (Org2)
			event, err = bob.EventsView("UpdateAsset", "UpdateAsset")
			Expect(err).ToNot(HaveOccurred())
			eventReceived = &views.EventReceived{}
			json.Unmarshal(event.([]byte), eventReceived)
			Expect(string(eventReceived.Event.Payload)).To(Equal("Invoked Update Asset Successfully"))
		})

		It("client listening to multiple chaincode events ", func() {
			expectedEventPayloads := []string{"Invoked Create Asset Successfully", "Invoked Update Asset Successfully"}
			var payloadsReceived []string
			// - Operate from Alice (Org1)
			events, err := alice.MultipleEventsView([]string{"CreateAsset", "UpdateAsset"}, 2)
			Expect(err).ToNot(HaveOccurred())
			eventsReceived := &views.MultipleEventsReceived{}
			err = json.Unmarshal(events.([]byte), eventsReceived)

			Expect(err).ToNot(HaveOccurred())

			for _, event := range eventsReceived.Events {
				payloadsReceived = append(payloadsReceived, string(event.Payload))
			}
			Expect(len(eventsReceived.Events)).To(Equal(2))
			Expect(payloadsReceived).To(Equal(expectedEventPayloads))

		})

		It("Upgrade Chaincode", func() {
			// Old chaincode
			event, err := alice.EventsView("CreateAsset", "CreateAsset")
			Expect(err).ToNot(HaveOccurred())
			eventReceived := &views.EventReceived{}
			json.Unmarshal(event.([]byte), eventReceived)
			Expect(string(eventReceived.Event.Payload)).To(Equal("Invoked Create Asset Successfully"))

			// Update
			fabricNetwork := fabric.Network(ii.Ctx, "default")
			Expect(fabricNetwork).ToNot(BeNil(), "failed to find fabric network 'default'")
			fabricNetwork.UpdateChaincode("events", "Version-1.0", "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events/chaincode/newChaincode", "")

			// New chaincode
			event, err = alice.EventsView("CreateAsset", "CreateAsset")
			Expect(err).ToNot(HaveOccurred())
			eventReceived = &views.EventReceived{}
			json.Unmarshal(event.([]byte), eventReceived)
			Expect(string(eventReceived.Event.Payload)).To(Equal("Invoked Create Asset Successfully From Upgraded Chaincode"))
		})
	})
})
