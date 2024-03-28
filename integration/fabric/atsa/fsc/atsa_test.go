/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc_test

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc"
	fsc2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/states"
)

type node = [2]string

var _ = Describe("EndToEnd", func() {
	Describe("Asset Transfer Secured Agreement (With Approvers) with LibP2P", func() {
		s := NewTestSuite(fsc2.LibP2P, &integration.NodeOptions{})
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", func() {
			s.TestSucceeded(node{"issuer", "issuer"}, node{"alice", "alice"}, node{"bob", "bob"}, node{"alice", "alice"})
		})
	})

	Describe("Asset Transfer Secured Agreement (With Approvers) with Websockets", func() {
		s := NewTestSuite(fsc2.WebSocket, &integration.NodeOptions{})
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", func() {
			s.TestSucceeded(node{"issuer", "issuer"}, node{"alice", "alice"}, node{"bob", "bob"}, node{"alice", "alice"})
		})
	})

	Describe("Asset Transfer Secured Agreement (With Approvers) with Websockets with replicas", func() {
		s := NewTestSuite(
			fsc2.WebSocket,
			&integration.NodeOptions{
				ReplicationFactors: map[string]int{
					"issuer":    2,
					"alice":     3,
					"bob":       2,
					"approvers": 2,
				},
				SQLConfigs: map[string]*sql.PostgresConfig{
					"alice": sql.DefaultConfig("alice-db"),
				},
			})

		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded 1", func() {
			s.TestSucceeded(node{"issuer", "fsc.issuer.0"}, node{"alice", "fsc.alice.0"}, node{"bob", "fsc.bob.0"}, node{"alice", "fsc.alice.1"})
		})
		It("succeeded 2", func() {
			s.TestSucceeded(node{"issuer", "fsc.issuer.1"}, node{"alice", "fsc.alice.1"}, node{"bob", "fsc.bob.1"}, node{"alice", "fsc.alice.1"})
		})
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType fsc2.P2PCommunicationType, nodeOpts *integration.NodeOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(nodeOpts.SQLConfigs, func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), true, fsc.Topology(&fabric.SDK{}, commType, nodeOpts)...)
	})}
}

func (s *TestSuite) TestSucceeded(issuerId node, sellerId node, buyerId node, transfererId node) {
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

	transferer := client.New(s.II.Client(transfererId[1]), s.II.Identity(transfererId[0]), approver)
	err = transferer.Transfer("1234", agreementID, s.II.Identity(buyerId[0]))
	Expect(err).ToNot(HaveOccurred())
}
