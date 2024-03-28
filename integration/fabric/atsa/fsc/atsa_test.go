/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc_test

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/states"
	fsc2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type node = [2]string

var _ = Describe("EndToEnd", func() {
	Describe("Asset Transfer Secured Agreement (With Approvers) with LibP2P", func() {
		s := TestSuite{commType: fsc2.LibP2P}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", func() {
			s.TestSucceeded(node{"issuer", "issuer"}, node{"alice", "alice"}, node{"bob", "bob"}, node{"alice", "alice"})
		})
	})

	Describe("Asset Transfer Secured Agreement (With Approvers) with Websockets", func() {
		s := TestSuite{commType: fsc2.WebSocket}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", func() {
			s.TestSucceeded(node{"issuer", "issuer"}, node{"alice", "alice"}, node{"bob", "bob"}, node{"alice", "alice"})
		})
	})

	Describe("Asset Transfer Secured Agreement (With Approvers) with Websockets with replicas", func() {
		s := TestSuite{
			commType: fsc2.WebSocket,
			replicas: map[string]int{
				"alice":     2,
				"bob":       2,
				"approvers": 2,
			},
			sqlConfigs: map[string]*sql.PostgresConfig{
				"alice": sql.DefaultConfig("alice-db"),
			},
		}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("succeeded", func() {
			s.TestSucceeded(node{"fsc.issuer.0", "issuer"}, node{"fsc.alice.0", "alice"}, node{"fsc.bob.0", "bob"}, node{"fsc.alice.1", "alice"})
		})
	})
})

type TestSuite struct {
	commType   fsc2.P2PCommunicationType
	replicas   map[string]int
	sqlConfigs map[string]*sql.PostgresConfig

	closeFunc func()
	ii        *integration.Infrastructure
}

func (s *TestSuite) TearDown() {
	s.ii.Stop()
	s.closeFunc()
}

func (s *TestSuite) Setup() {
	closeFunc, err := sql.StartPostgresWithFmt(s.sqlConfigs)
	Expect(err).NotTo(HaveOccurred())
	s.closeFunc = closeFunc
	// Create the integration ii
	ii, err := integration.Generate(StartPort(), true, fsc.Topology(&fabric.SDK{}, s.commType, s.replicas, s.sqlConfigs)...)
	Expect(err).NotTo(HaveOccurred())
	s.ii = ii
	// Start the integration ii
	ii.Start()
}

func (s *TestSuite) TestSucceeded(issuerId node, sellerId node, buyerId node, transferringUserId node) {
	approver := s.ii.Identity("approver")
	issuer := client.New(s.ii.Client(issuerId[0]), s.ii.Identity(issuerId[1]), approver)
	txID, err := issuer.Issue(&states.Asset{
		ObjectType:        "coin",
		ID:                "1234",
		Owner:             s.ii.Identity(sellerId[1]),
		PublicDescription: "Coin",
		PrivateProperties: []byte("Hello World!!!"),
	})
	Expect(err).ToNot(HaveOccurred())

	seller := client.New(s.ii.Client(sellerId[0]), s.ii.Identity(sellerId[1]), approver)
	Expect(seller.IsTxFinal(txID)).NotTo(HaveOccurred())
	agreementID, err := seller.AgreeToSell(&states.AgreementToSell{
		TradeID: "1234",
		ID:      "1234",
		Price:   100,
	})
	Expect(err).ToNot(HaveOccurred())

	buyer := client.New(s.ii.Client(buyerId[0]), s.ii.Identity(buyerId[1]), approver)
	_, err = buyer.AgreeToBuy(&states.AgreementToBuy{
		TradeID: "1234",
		ID:      "1234",
		Price:   100,
	})
	Expect(err).ToNot(HaveOccurred())

	transferringUser := client.New(s.ii.Client(transferringUserId[0]), s.ii.Identity(transferringUserId[1]), approver)
	err = transferringUser.Transfer("1234", agreementID, s.ii.Identity(buyerId[1]))
	Expect(err).ToNot(HaveOccurred())
}
