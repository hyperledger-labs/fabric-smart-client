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
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fabric2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EndToEnd", func() {
	Describe("Events (With Chaincode) With LibP2P", func() {
		s := TestSuite{commType: fsc.LibP2P}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("clients listening to single chaincode events", s.TestSingleChaincodeEvents)
		It("client listening to multiple chaincode events ", s.TestMultipleChaincodeEvents)
		It("Upgrade Chaincode", s.TestUpgradeChaincode)
	})

	Describe("Events (With Chaincode) With Websockets", func() {
		s := TestSuite{commType: fsc.WebSocket}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("clients listening to single chaincode events", s.TestSingleChaincodeEvents)
		It("client listening to multiple chaincode events ", s.TestMultipleChaincodeEvents)
		It("Upgrade Chaincode", s.TestUpgradeChaincode)
	})
})

type TestSuite struct {
	commType fsc.P2PCommunicationType
	replicas map[string]int

	ii *integration.Infrastructure

	alice *chaincode.Client
	bob   *chaincode.Client
}

func (s *TestSuite) TearDown() {
	s.ii.Stop()
}

func (s *TestSuite) Setup() {
	// Create the integration ii
	ii, err := integration.Generate(StartPort(), true, chaincode.Topology(&fabric2.SDK{}, s.commType, s.replicas)...)
	Expect(err).NotTo(HaveOccurred())
	s.ii = ii
	// Start the integration ii
	ii.Start()

	s.alice = chaincode.NewClient(ii.Client("alice"), ii.Identity("alice"))
	s.bob = chaincode.NewClient(ii.Client("bob"), ii.Identity("bob"))
}

func (s *TestSuite) TestSingleChaincodeEvents() {
	alice := s.alice
	bob := s.bob
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
}

func (s *TestSuite) TestMultipleChaincodeEvents() {
	expectedEventPayloads := []string{"Invoked Create Asset Successfully", "Invoked Update Asset Successfully"}
	var payloadsReceived []string
	// - Operate from Alice (Org1)
	events, err := s.alice.MultipleEventsView([]string{"CreateAsset", "UpdateAsset"}, 2)
	Expect(err).ToNot(HaveOccurred())
	eventsReceived := &views.MultipleEventsReceived{}
	err = json.Unmarshal(events.([]byte), eventsReceived)

	Expect(err).ToNot(HaveOccurred())

	for _, event := range eventsReceived.Events {
		payloadsReceived = append(payloadsReceived, string(event.Payload))
	}
	Expect(len(eventsReceived.Events)).To(Equal(2))
	Expect(payloadsReceived).To(Equal(expectedEventPayloads))
}

func (s *TestSuite) TestUpgradeChaincode() {
	// Old chaincode
	event, err := s.alice.EventsView("CreateAsset", "CreateAsset")
	Expect(err).ToNot(HaveOccurred())
	eventReceived := &views.EventReceived{}
	json.Unmarshal(event.([]byte), eventReceived)
	Expect(string(eventReceived.Event.Payload)).To(Equal("Invoked Create Asset Successfully"))

	// Update
	fabricNetwork := fabric.Network(s.ii.Ctx, "default")
	Expect(fabricNetwork).ToNot(BeNil(), "failed to find fabric network 'default'")
	fabricNetwork.UpdateChaincode("events", "Version-1.0", "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events/chaincode/newChaincode", "")

	// New chaincode
	event, err = s.alice.EventsView("CreateAsset", "CreateAsset")
	Expect(err).ToNot(HaveOccurred())
	eventReceived = &views.EventReceived{}
	json.Unmarshal(event.([]byte), eventReceived)
	Expect(string(eventReceived.Event.Payload)).To(Equal("Invoked Create Asset Successfully From Upgraded Chaincode"))
}
