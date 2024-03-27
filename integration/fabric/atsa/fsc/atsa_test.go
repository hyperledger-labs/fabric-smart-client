/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc_test

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc"
	fsc2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/states"
)

var _ = Describe("EndToEnd", func() {
	Describe("Asset Transfer Secured Agreement (With Approvers) with LibP2P", func() {
		s := TestSuite{commType: fsc2.LibP2P}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("Asset Transfer Secured Agreement (With Approvers) with Websockets", func() {
		s := TestSuite{commType: fsc2.WebSocket}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})
})

type TestSuite struct {
	commType fsc2.P2PCommunicationType
	replicas map[string]int

	ii *integration.Infrastructure

	issuer *client.Client
	alice  *client.Client
	bob    *client.Client
}

func (s *TestSuite) TearDown() {
	s.ii.Stop()
}

func (s *TestSuite) Setup() {
	// Create the integration ii
	ii, err := integration.Generate(StartPort(), true, fsc.Topology(&fabric.SDK{}, s.commType, s.replicas)...)
	Expect(err).NotTo(HaveOccurred())
	s.ii = ii
	// Start the integration ii
	ii.Start()

	approver := ii.Identity("approver")

	s.issuer = client.New(ii.Client("issuer"), ii.Identity("issuer"), approver)
	s.alice = client.New(ii.Client("alice"), ii.Identity("alice"), approver)
	s.bob = client.New(ii.Client("bob"), ii.Identity("bob"), approver)
}

func (s *TestSuite) TestSucceeded() {
	txID, err := s.issuer.Issue(&states.Asset{
		ObjectType:        "coin",
		ID:                "1234",
		Owner:             s.ii.Identity("alice"),
		PublicDescription: "Coin",
		PrivateProperties: []byte("Hello World!!!"),
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(s.alice.IsTxFinal(txID)).NotTo(HaveOccurred())

	agreementID, err := s.alice.AgreeToSell(&states.AgreementToSell{
		TradeID: "1234",
		ID:      "1234",
		Price:   100,
	})
	Expect(err).ToNot(HaveOccurred())

	_, err = s.bob.AgreeToBuy(&states.AgreementToBuy{
		TradeID: "1234",
		ID:      "1234",
		Price:   100,
	})
	Expect(err).ToNot(HaveOccurred())

	err = s.alice.Transfer("1234", agreementID, s.ii.Identity("bob"))
	Expect(err).ToNot(HaveOccurred())
}
