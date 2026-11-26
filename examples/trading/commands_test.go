/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package trading_test

import (
	"context"
	"time"

	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/examples/trading/states"
	"github.com/hyperledger-labs/fabric-smart-client/examples/trading/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const viewTimeout = 2 * time.Minute

var producers = []string{"mine", "forest", "refinery", "mill"}

// call runs the view registered as fid on node and returns its raw result.
func call(ii *integration.Infrastructure, node, fid string, in any) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
	defer cancel()
	res, err := ii.Client(node).CallViewWithContext(ctx, fid, common.JSONMarshall(in))
	if err != nil {
		return nil, err
	}
	raw, ok := res.([]byte)
	Expect(ok).To(BeTrue(), "unexpected view result type %T", res)
	return raw, nil
}

// SetupMarket registers the four producers with their roles and 1000 coins
// each, and returns each producer's Fabric identity by node name.
func SetupMarket(ii *integration.Infrastructure) map[string]view.Identity {
	roles := map[string]states.Role{
		"mine":     states.GoldProducer,
		"forest":   states.WoodProducer,
		"refinery": states.GoldRefiner,
		"mill":     states.WoodRefiner,
	}
	ids := map[string]view.Identity{}
	setup := &views.Setup{Coins: 1000}
	for _, name := range producers {
		raw, err := call(ii, name, "whoami", nil)
		Expect(err).NotTo(HaveOccurred())
		var id view.Identity
		common.JSONUnmarshal(raw, &id)
		ids[name] = id
		setup.Producers = append(setup.Producers, views.Producer{Identity: id, Role: roles[name]})
	}
	_, err := call(ii, "market", "setup", setup)
	Expect(err).NotTo(HaveOccurred())
	return ids
}

// State reads the committed state under key into st, and returns false if
// there is none.
func State(ii *integration.Infrastructure, key string, st any) bool {
	raw, err := call(ii, "mine", "state", &views.QueryState{Key: key})
	if err != nil {
		Expect(err).To(MatchError(ContainSubstring("not found")))
		return false
	}
	common.JSONUnmarshal(raw, st)
	return true
}

func Balance(ii *integration.Infrastructure, owner view.Identity) uint64 {
	b := &states.Balance{}
	Expect(State(ii, states.BalanceID(owner), b)).To(BeTrue())
	return b.Coins
}

func Lot(ii *integration.Infrastructure, lotID string) *states.Lot {
	l := &states.Lot{}
	Expect(State(ii, lotID, l)).To(BeTrue(), "lot %s", lotID)
	return l
}

func Listing(ii *integration.Infrastructure, listingID string) *states.Listing {
	l := &states.Listing{}
	Expect(State(ii, listingID, l)).To(BeTrue(), "listing %s", listingID)
	return l
}

// Browse asks the market, from node, for open listings of kind and qty.
func Browse(ii *integration.Infrastructure, node string, kind states.Kind, qty uint64) []*states.Listing {
	raw, err := call(ii, node, "browse", &views.Browse{Kind: kind, Qty: qty, Market: ii.Identity("market")})
	Expect(err).NotTo(HaveOccurred())
	var listings []*states.Listing
	common.JSONUnmarshal(raw, &listings)
	return listings
}

func Produce(ii *integration.Infrastructure, node string, kind states.Kind, qty uint64) (string, error) {
	raw, err := call(ii, node, "produce", &views.Produce{Kind: kind, Qty: qty, Market: ii.Identity("market")})
	if err != nil {
		return "", err
	}
	return common.JSONUnmarshalString(raw), nil
}

func Refine(ii *integration.Infrastructure, node, lotID string) string {
	raw, err := call(ii, node, "refine", &views.Refine{LotID: lotID, Market: ii.Identity("market")})
	Expect(err).NotTo(HaveOccurred())
	return common.JSONUnmarshalString(raw)
}

func List(ii *integration.Infrastructure, node, lotID string, ask, floor, step uint64) string {
	raw, err := call(ii, node, "list", &views.List{LotID: lotID, Ask: ask, Floor: floor, Step: step, Market: ii.Identity("market")})
	Expect(err).NotTo(HaveOccurred())
	return common.JSONUnmarshalString(raw)
}

func BuyGoods(ii *integration.Infrastructure, node string, b *views.Buy) (*views.BuyResult, error) {
	b.Market = ii.Identity("market")
	raw, err := call(ii, node, "buy", b)
	if err != nil {
		return nil, err
	}
	r := &views.BuyResult{}
	common.JSONUnmarshal(raw, r)
	return r, nil
}
