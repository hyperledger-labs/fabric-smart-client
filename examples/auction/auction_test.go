/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package auction_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/examples/auction"
	"github.com/hyperledger-labs/fabric-smart-client/examples/auction/states"
	"github.com/hyperledger-labs/fabric-smart-client/examples/auction/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

var _ = Describe("EndToEnd", func() {
	Describe("Sealed-bid auction", Label(nwofsc.WebSocket), func() {
		s := NewTestSuite(nwofsc.WebSocket)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)

		It("runs an auction from creation to an audited reveal", s.TestAuction)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType nwofsc.P2PCommunicationType) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.New(integration.FabricXAuctionPort.StartPortForNode(), "", auction.Topology(&auction.SDK{}, commType)...)
		if err != nil {
			return nil, err
		}
		ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())
		ii.Generate()
		return ii, nil
	})}
}

func (s *TestSuite) TestAuction() {
	Init(s.II)

	By("the house opens an auction")
	auctionID := Create(s.II, "painting")
	a := Query(s.II, "alice", auctionID)
	Expect(a.Status).To(Equal(states.Open))
	Expect(a.Item).To(Equal("painting"))

	By("each bidder places a sealed bid")
	bids := map[string]string{}
	for bidder, price := range map[string]uint64{"alice": 10, "bob": 30, "charlie": 20} {
		bidID, err := Bid(s.II, bidder, auctionID, price)
		Expect(err).NotTo(HaveOccurred(), "bid of %s", bidder)
		bids[bidder] = bidID
	}
	Expect(bids["alice"]).NotTo(Equal(bids["bob"]))
	Expect(bids["bob"]).NotTo(Equal(bids["charlie"]))

	By("a bid on an auction that does not exist fails")
	_, err := Bid(s.II, "alice", "no-such-auction", 5)
	Expect(err).To(MatchError(ContainSubstring("failed reading auction [no-such-auction]")))

	By("nobody has won yet")
	Expect(AuditBid(s.II, "alice", auctionID, bids["alice"])).To(Equal(views.NotEnded))

	By("the house closes the auction; later bids fail")
	Close(s.II, auctionID)
	_, err = Bid(s.II, "charlie", auctionID, 50)
	Expect(err).To(MatchError(ContainSubstring("is not open")))

	By("the house reveals the winning bid")
	winner := Reveal(s.II, auctionID)
	Expect(winner.ID).To(Equal(bids["bob"]))
	Expect(winner.Price).To(BeEquivalentTo(30))
	a = Query(s.II, "alice", auctionID)
	Expect(a.Status).To(Equal(states.Ended))
	Expect(a.Winner.Equal(winner.Bidder)).To(BeTrue())

	By("each bidder checks the reveal against the ledger")
	Expect(AuditBid(s.II, "bob", auctionID, bids["bob"])).To(Equal(views.Won))
	Expect(AuditBid(s.II, "alice", auctionID, bids["alice"])).To(Equal(views.Lost))
	Expect(AuditBid(s.II, "charlie", auctionID, bids["charlie"])).To(Equal(views.Lost))
}
