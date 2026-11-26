/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package trading_test

import (
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/examples/trading"
	"github.com/hyperledger-labs/fabric-smart-client/examples/trading/states"
	"github.com/hyperledger-labs/fabric-smart-client/examples/trading/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

var _ = Describe("EndToEnd", func() {
	Describe("Trading", Label(nwofsc.WebSocket), func() {
		s := NewTestSuite(nwofsc.WebSocket)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)

		It("trades goods along the whole chain", s.TestTrading)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType nwofsc.P2PCommunicationType) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.New(integration.FabricXTradingPort.StartPortForNode(), "", trading.Topology(&trading.SDK{}, commType)...)
		if err != nil {
			return nil, err
		}
		ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())
		ii.Generate()
		return ii, nil
	})}
}

func (s *TestSuite) TestTrading() {
	By("the market sets up the producers with 1000 coins each")
	ids := SetupMarket(s.II)
	for _, name := range producers {
		Expect(Balance(s.II, ids[name])).To(BeEquivalentTo(1000), name)
	}
	_, err := call(s.II, "market", "setup", &views.Setup{Coins: 1})
	Expect(err).To(MatchError(ContainSubstring("already set up")))

	produce := func(node string, kind states.Kind, qty uint64) string {
		lotID, err := Produce(s.II, node, kind, qty)
		Expect(err).NotTo(HaveOccurred())
		return lotID
	}

	By("the mine produces gold and lists it")
	gold := produce("mine", states.Gold, 10)
	_, err = Produce(s.II, "forest", states.Gold, 10)
	Expect(err).To(MatchError(ContainSubstring("produces wood, not gold")))
	goldListing := List(s.II, "mine", gold, 100, 90, 5)
	Expect(Browse(s.II, "refinery", states.Gold, 10)).To(ConsistOf(HaveField("ID", goldListing)))
	Expect(Browse(s.II, "refinery", states.Gold, 11)).To(BeEmpty())

	buy := func(node string, kind states.Kind, qty, opening, budget, step uint64) *views.BuyResult {
		r, err := BuyGoods(s.II, node, &views.Buy{Kind: kind, Qty: qty, Opening: opening, Budget: budget, Step: step})
		Expect(err).NotTo(HaveOccurred())
		return r
	}

	By("the refinery negotiates the gold down from 100 to 90 and buys it")
	Expect(buy("refinery", states.Gold, 10, 80, 100, 10)).To(Equal(&views.BuyResult{Deal: true, ListingID: goldListing, Price: 90}))
	Expect(Lot(s.II, gold).Owner).To(Equal(ids["refinery"]))
	Expect(Listing(s.II, goldListing).Status).To(Equal(states.Sold))

	By("the refinery makes bars and sells them to the mill")
	bars := Refine(s.II, "refinery", gold)
	Expect(Lot(s.II, bars)).To(Equal(&states.Lot{ID: bars, Kind: states.Bar, Qty: 5, Owner: ids["refinery"]}))
	Expect(State(s.II, gold, &states.Lot{})).To(BeFalse(), "the gold is used up")
	List(s.II, "refinery", bars, 200, 180, 10)
	Expect(buy("mill", states.Bar, 5, 150, 200, 20).Price).To(BeEquivalentTo(180))

	By("the forest sells wood to the mill")
	wood := produce("forest", states.Wood, 20)
	List(s.II, "forest", wood, 60, 50, 5)
	Expect(buy("mill", states.Wood, 20, 40, 60, 5).Price).To(BeEquivalentTo(50))

	By("the mill makes planks and sells them to the refinery")
	planks := Refine(s.II, "mill", wood)
	List(s.II, "mill", planks, 150, 130, 10)
	Expect(buy("refinery", states.Plank, 10, 120, 150, 5).Price).To(BeEquivalentTo(130))

	expected := map[string]uint64{"mine": 1090, "forest": 1050, "refinery": 960, "mill": 900}
	for name, coins := range expected {
		Expect(Balance(s.II, ids[name])).To(Equal(coins), name)
	}

	By("a negotiation that ends without a deal leaves everything as it was")
	small := produce("mine", states.Gold, 4)
	noDeal := List(s.II, "mine", small, 100, 90, 5)
	// Twice: the second attempt finds the listing free again, not busy.
	for range 2 {
		Expect(buy("refinery", states.Gold, 4, 60, 70, 5)).To(Equal(&views.BuyResult{Reason: "no deal"}))
	}
	Expect(Listing(s.II, noDeal).Status).To(Equal(states.Open))
	Expect(Lot(s.II, small).Owner).To(Equal(ids["mine"]))
	for name, coins := range expected {
		Expect(Balance(s.II, ids[name])).To(Equal(coins), name)
	}

	By("the refinery and the mill race for the same gold; exactly one gets it")
	contested := produce("mine", states.Gold, 6)
	race := List(s.II, "mine", contested, 100, 90, 5)
	racers := []string{"refinery", "mill"}
	results := make([]*views.BuyResult, len(racers))
	var wg sync.WaitGroup
	for i, node := range racers {
		wg.Go(func() {
			defer GinkgoRecover()
			results[i] = buy(node, states.Gold, 6, 80, 100, 10)
		})
	}
	wg.Wait()
	winner, loser := 0, 1
	if !results[0].Deal {
		winner, loser = 1, 0
	}
	Expect(results[winner]).To(Equal(&views.BuyResult{Deal: true, ListingID: race, Price: 90}))
	Expect(results[loser].Deal).To(BeFalse())
	Expect(results[loser].Reason).To(BeElementOf("busy", "not open", "no open listing"))
	Expect(Lot(s.II, contested).Owner).To(Equal(ids[racers[winner]]))
	expected["mine"] += 90
	expected[racers[winner]] -= 90

	By("every coin and every lot is accounted for")
	var total uint64
	for name, coins := range expected {
		Expect(Balance(s.II, ids[name])).To(Equal(coins), name)
		total += coins
	}
	Expect(total).To(BeEquivalentTo(4000))
	Expect(expected["mine"]).To(BeEquivalentTo(1180))
	owners := map[string]string{bars: "mill", planks: "refinery", small: "mine", contested: racers[winner]}
	for lotID, owner := range owners {
		Expect(Lot(s.II, lotID).Owner).To(Equal(ids[owner]), lotID)
	}
	Expect(State(s.II, wood, &states.Lot{})).To(BeFalse(), "the wood is used up")
	Expect(Listing(s.II, noDeal).Status).To(Equal(states.Open))
	Expect(Browse(s.II, "refinery", states.Gold, 4)).To(ConsistOf(HaveField("ID", noDeal)))

	By("a listing whose lot was sold through another listing leaves the board")
	first := List(s.II, "mill", bars, 200, 180, 10)
	second := List(s.II, "mill", bars, 200, 180, 10)
	Expect(Browse(s.II, "refinery", states.Bar, 5)).To(HaveLen(2))
	r := buy("refinery", states.Bar, 5, 150, 200, 20)
	Expect(r.Deal).To(BeTrue())
	Expect(r.ListingID).To(BeElementOf(first, second))
	Expect(Browse(s.II, "refinery", states.Bar, 5)).To(BeEmpty())
	Expect(buy("refinery", states.Bar, 5, 150, 200, 20)).To(Equal(&views.BuyResult{Reason: "no open listing"}))
}
