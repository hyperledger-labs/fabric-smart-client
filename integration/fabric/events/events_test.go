/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package events_test

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fabricsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EndToEnd", func() {
	Describe("Events (With Chaincode) With Websockets", func() {
		s := NewTestSuite(fsc.WebSocket, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("clients listening to single chaincode events", s.TestSingleChaincodeEvents)
		It("client listening to multiple chaincode events", s.TestMultipleChaincodeEvents)
		It("multiple clients unsubscribing", s.TestMultipleListenersAndUnsubscribe)
		It("Upgrade Chaincode", s.TestUpgradeChaincode)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType fsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), integration.WithRaceDetection, events.Topology(&fabricsdk.SDK{}, commType, nodeOpts)...)
	})}
}

func (s *TestSuite) TestSingleChaincodeEvents() {
	alice := events.NewClient(s.II.Client("alice"), s.II.Identity("alice"))
	bob := events.NewClient(s.II.Client("bob"), s.II.Identity("bob"))
	// - Operate from Alice (Org1)

	event, err := alice.EventsView("CreateAsset", "CreateAsset")

	Expect(err).ToNot(HaveOccurred())
	eventReceived := &views.EventReceived{}
	Expect(json.Unmarshal(event.([]byte), eventReceived)).ToNot(HaveOccurred())
	Expect(string(eventReceived.Event.Payload)).To(Equal("Invoked Create Asset Successfully"))

	// - Operate from Bob (Org2)
	event, err = bob.EventsView("UpdateAsset", "UpdateAsset")
	Expect(err).ToNot(HaveOccurred())
	eventReceived = &views.EventReceived{}
	Expect(json.Unmarshal(event.([]byte), eventReceived)).ToNot(HaveOccurred())
	Expect(string(eventReceived.Event.Payload)).To(Equal("Invoked Update Asset Successfully"))
}

func (s *TestSuite) TestMultipleChaincodeEvents() {
	alice := events.NewClient(s.II.Client("alice"), s.II.Identity("alice"))

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
}

func (s *TestSuite) TestMultipleListenersAndUnsubscribe() {
	n := 20
	alice := events.NewClient(s.II.Client("alice"), s.II.Identity("alice"))

	// - Operate from Alice (Org1)
	events, err := alice.MultipleListenersView("CreateAsset", "CreateAsset", n)
	Expect(err).ToNot(HaveOccurred())

	eventsReceived := &views.MultipleEventsReceived{}
	err = json.Unmarshal(events.([]byte), eventsReceived)
	Expect(err).ToNot(HaveOccurred())

	Expect(len(eventsReceived.Events)).To(Equal(n))
	for _, event := range eventsReceived.Events {
		Expect(event).To(Not(BeNil()))
		Expect(string(event.Payload)).To(Equal("Invoked Create Asset Successfully"))
	}
}

func (s *TestSuite) TestUpgradeChaincode() {
	alice := events.NewClient(s.II.Client("alice"), s.II.Identity("alice"))
	// Old chaincode
	event, err := alice.EventsView("CreateAsset", "CreateAsset")
	Expect(err).ToNot(HaveOccurred())
	eventReceived := &views.EventReceived{}
	Expect(json.Unmarshal(event.([]byte), eventReceived)).ToNot(HaveOccurred())
	Expect(string(eventReceived.Event.Payload)).To(Equal("Invoked Create Asset Successfully"))

	// Update
	fabricNetwork := fabric.Network(s.II.Ctx, "default")
	Expect(fabricNetwork).ToNot(BeNil(), "failed to find fabric network 'default'")
	fabricNetwork.UpdateChaincode("events", "Version-1.0", "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events/chaincode2", "")

	// New chaincode
	event, err = alice.EventsView("CreateAsset", "CreateAsset")
	Expect(err).ToNot(HaveOccurred())
	eventReceived = &views.EventReceived{}
	Expect(json.Unmarshal(event.([]byte), eventReceived)).ToNot(HaveOccurred())
	Expect(string(eventReceived.Event.Payload)).To(Equal("Invoked Create Asset Successfully From Upgraded Chaincode"))
}
