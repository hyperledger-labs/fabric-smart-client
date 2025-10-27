/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc_test

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	atsa "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/states"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type node = [2]string

var _ = Describe("EndToEnd", func() {
	Describe("Asset Transfer Secured Agreement (With Approvers) with LibP2P", func() {
		s := NewTestSuite(nwofsc.LibP2P, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("Asset Transfer Secured Agreement (With Approvers) with Websockets", func() {
		s := NewTestSuite(nwofsc.WebSocket, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", s.TestSucceeded)
	})

	Describe("Asset Transfer Secured Agreement (With Approvers) with Websockets with replicas", func() {
		s := NewTestSuite(
			nwofsc.WebSocket,
			&integration.ReplicationOptions{
				ReplicationFactors: map[string]int{
					"issuer":    2,
					"alice":     3,
					"bob":       2,
					"approvers": 2,
				},
				SQLConfigs: map[string]*postgres.ContainerConfig{
					"alice": postgres.DefaultConfig(postgres.WithDBName("alice-db")),
					"bob":   postgres.DefaultConfig(postgres.WithDBName("bob-db")),
				},
			})

		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded 1", func() {
			s.TestSucceededWithUsers(node{"issuer", "fsc.issuer.0"}, node{"alice", "fsc.alice.0"}, node{"bob", "fsc.bob.0"}, node{"alice", "fsc.alice.1"})
		})
		It("succeeded 2", func() {
			s.TestSucceededWithUsers(node{"issuer", "fsc.issuer.1"}, node{"alice", "fsc.alice.1"}, node{"bob", "fsc.bob.1"}, node{"alice", "fsc.alice.1"})
		})
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType nwofsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), integration.WithRaceDetection, atsa.Topology(&atsa.SDK{}, commType, nodeOpts)...)
	})}
}

func (s *TestSuite) TestSucceeded() {
	s.TestSucceededWithUsers(node{"issuer", "issuer"}, node{"alice", "alice"}, node{"bob", "bob"}, node{"alice", "alice"})
}

func (s *TestSuite) TestSucceededWithUsers(issuerId node, sellerId node, buyerId node, transferringUserId node) {
	approver := s.II.Identity("approver")

	issuer := client.New(s.II.Client(issuerId[1]), s.II.Identity(issuerId[0]), approver)
	txID, err := issuer.Issue(&states.Asset{
		ObjectType:        "coin",
		ID:                "1234",
		Owner:             s.II.Identity(sellerId[0]),
		PublicDescription: "Coin",
		PrivateProperties: []byte("Hello World!!!"),
	})
	Expect(err).ToNot(HaveOccurred())

	seller := client.New(s.II.Client(sellerId[1]), s.II.Identity(sellerId[0]), approver)
	Expect(seller.IsTxFinal(txID)).NotTo(HaveOccurred())
	agreementID, err := seller.AgreeToSell(&states.AgreementToSell{
		TradeID: "1234",
		ID:      "1234",
		Price:   100,
	})
	Expect(err).ToNot(HaveOccurred())

	buyer := client.New(s.II.Client(buyerId[1]), s.II.Identity(buyerId[0]), approver)
	_, err = buyer.AgreeToBuy(&states.AgreementToBuy{
		TradeID: "1234",
		ID:      "1234",
		Price:   100,
	})
	Expect(err).ToNot(HaveOccurred())

	transferringUser := client.New(s.II.Client(transferringUserId[1]), s.II.Identity(transferringUserId[0]), approver)
	err = transferringUser.Transfer("1234", agreementID, s.II.Identity(buyerId[0]))
	Expect(err).ToNot(HaveOccurred())
}
