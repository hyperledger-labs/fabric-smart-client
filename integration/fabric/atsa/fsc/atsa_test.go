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
		s := NewTestSuite(fsc2.LibP2P, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("Asset Transfer Secured Agreement (With Approvers) with Websockets", func() {
		s := NewTestSuite(fsc2.WebSocket, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType fsc2.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), true, fsc.Topology(&fabric.SDK{}, commType, nodeOpts)...)
	})}
}

func (s *TestSuite) TestSucceeded() {
	approver := s.II.Identity("approver")
	issuer := client.New(s.II.Client("issuer"), s.II.Identity("issuer"), approver)
	alice := client.New(s.II.Client("alice"), s.II.Identity("alice"), approver)
	bob := client.New(s.II.Client("bob"), s.II.Identity("bob"), approver)
	txID, err := issuer.Issue(&states.Asset{
		ObjectType:        "coin",
		ID:                "1234",
		Owner:             s.II.Identity("alice"),
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

	err = alice.Transfer("1234", agreementID, s.II.Identity("bob"))
	Expect(err).ToNot(HaveOccurred())
}
