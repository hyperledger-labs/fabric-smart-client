/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/examples/trading/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var (
	mine     = view.Identity("mine")
	refinery = view.Identity("refinery")
	mill     = view.Identity("mill")
)

func TestCheckProduce(t *testing.T) {
	t.Parallel()

	good := func() *states.Lot {
		return &states.Lot{ID: states.LotID("tx1"), Kind: states.Gold, Qty: 10, Owner: mine}
	}
	require.NoError(t, CheckProduce(states.GoldProducer, mine, "tx1", good()))

	for name, tc := range map[string]struct {
		role states.Role
		edit func(*states.Lot)
	}{
		"wrong kind":     {states.GoldProducer, func(l *states.Lot) { l.Kind = states.Wood }},
		"no quantity":    {states.GoldProducer, func(l *states.Lot) { l.Qty = 0 }},
		"someone else":   {states.GoldProducer, func(l *states.Lot) { l.Owner = refinery }},
		"wrong ID":       {states.GoldProducer, func(l *states.Lot) { l.ID = states.LotID("tx2") }},
		"not a producer": {states.GoldRefiner, func(*states.Lot) {}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			lot := good()
			tc.edit(lot)
			require.Error(t, CheckProduce(tc.role, mine, "tx1", lot))
		})
	}
}

func TestCheckRefine(t *testing.T) {
	t.Parallel()

	raw := func() *states.Lot {
		return &states.Lot{ID: states.LotID("tx0"), Kind: states.Gold, Qty: 10, Owner: refinery}
	}
	refined := func() *states.Lot {
		return &states.Lot{ID: states.LotID("tx1"), Kind: states.Bar, Qty: 5, Owner: refinery}
	}
	require.NoError(t, CheckRefine(states.GoldRefiner, refinery, "tx1", raw(), refined()))

	for name, tc := range map[string]struct {
		role    states.Role
		raw     func(*states.Lot)
		refined func(*states.Lot)
	}{
		"odd quantity":       {states.GoldRefiner, func(l *states.Lot) { l.Qty = 9 }, func(l *states.Lot) { l.Qty = 4 }},
		"no quantity":        {states.GoldRefiner, func(l *states.Lot) { l.Qty = 0 }, func(l *states.Lot) { l.Qty = 0 }},
		"raw not mine":       {states.GoldRefiner, func(l *states.Lot) { l.Owner = mine }, func(*states.Lot) {}},
		"raw wrong kind":     {states.GoldRefiner, func(l *states.Lot) { l.Kind = states.Wood }, func(*states.Lot) {}},
		"too few refined":    {states.GoldRefiner, func(*states.Lot) {}, func(l *states.Lot) { l.Qty = 4 }},
		"refined wrong kind": {states.GoldRefiner, func(*states.Lot) {}, func(l *states.Lot) { l.Kind = states.Plank }},
		"refined for mill":   {states.GoldRefiner, func(*states.Lot) {}, func(l *states.Lot) { l.Owner = mill }},
		"refined wrong ID":   {states.GoldRefiner, func(*states.Lot) {}, func(l *states.Lot) { l.ID = states.LotID("tx2") }},
		"not a refiner":      {states.GoldProducer, func(*states.Lot) {}, func(*states.Lot) {}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r, o := raw(), refined()
			tc.raw(r)
			tc.refined(o)
			require.Error(t, CheckRefine(tc.role, refinery, "tx1", r, o))
		})
	}
}

func TestCheckList(t *testing.T) {
	t.Parallel()

	lot := func() *states.Lot {
		return &states.Lot{ID: states.LotID("tx0"), Kind: states.Gold, Qty: 10, Owner: mine}
	}
	listing := func() *states.Listing {
		return &states.Listing{
			ID: states.ListingID("tx1"), LotID: states.LotID("tx0"), Kind: states.Gold, Qty: 10,
			Seller: mine, Ask: 100, Status: states.Open,
		}
	}
	require.NoError(t, CheckList(mine, "tx1", lot(), listing()))

	for name, tc := range map[string]struct {
		lot     func(*states.Lot)
		listing func(*states.Listing)
	}{
		"lot not mine": {func(l *states.Lot) { l.Owner = refinery }, func(*states.Listing) {}},
		"other seller": {func(*states.Lot) {}, func(l *states.Listing) { l.Seller = refinery }},
		"other lot":    {func(*states.Lot) {}, func(l *states.Listing) { l.LotID = states.LotID("tx9") }},
		"kind differs": {func(*states.Lot) {}, func(l *states.Listing) { l.Kind = states.Bar }},
		"qty differs":  {func(*states.Lot) {}, func(l *states.Listing) { l.Qty = 9 }},
		"no ask":       {func(*states.Lot) {}, func(l *states.Listing) { l.Ask = 0 }},
		"already sold": {func(*states.Lot) {}, func(l *states.Listing) { l.Status = states.Sold }},
		"price set":    {func(*states.Lot) {}, func(l *states.Listing) { l.Price = 90 }},
		"wrong ID":     {func(*states.Lot) {}, func(l *states.Listing) { l.ID = states.ListingID("tx2") }},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			lt, ls := lot(), listing()
			tc.lot(lt)
			tc.listing(ls)
			require.Error(t, CheckList(mine, "tx1", lt, ls))
		})
	}
}

// goldSwap returns the states of the refinery buying the mine's gold for 90.
func goldSwap() (before, after SwapStates) {
	before = SwapStates{
		Listing: &states.Listing{
			ID: states.ListingID("tx1"), LotID: states.LotID("tx0"), Kind: states.Gold, Qty: 10,
			Seller: mine, Ask: 100, Status: states.Open,
		},
		Lot:    &states.Lot{ID: states.LotID("tx0"), Kind: states.Gold, Qty: 10, Owner: mine},
		Buyer:  &states.Balance{ID: states.BalanceID(refinery), Owner: refinery, Coins: 1000},
		Seller: &states.Balance{ID: states.BalanceID(mine), Owner: mine, Coins: 1000},
	}
	listing, lot, buyer, seller := *before.Listing, *before.Lot, *before.Buyer, *before.Seller
	listing.Status, listing.Price = states.Sold, 90
	lot.Owner = refinery
	buyer.Coins, seller.Coins = 910, 1090
	return before, SwapStates{Listing: &listing, Lot: &lot, Buyer: &buyer, Seller: &seller}
}

func TestCheckSwap(t *testing.T) {
	t.Parallel()

	before, after := goldSwap()
	require.NoError(t, CheckSwap(refinery, mine, before, after))

	for name, edit := range map[string]func(before, after *SwapStates){
		"already sold":      func(b, _ *SwapStates) { b.Listing.Status = states.Sold },
		"buyer cannot pay":  func(b, a *SwapStates) { b.Buyer.Coins = 50; a.Buyer.Coins = 0 },
		"no price":          func(_, a *SwapStates) { a.Listing.Price = 0; a.Buyer.Coins = 1000; a.Seller.Coins = 1000 },
		"seller underpaid":  func(_, a *SwapStates) { a.Seller.Coins = 1080 },
		"buyer overcharged": func(_, a *SwapStates) { a.Buyer.Coins = 900 },
		"lot changed":       func(_, a *SwapStates) { a.Lot.Qty = 9 },
		"lot not to buyer":  func(_, a *SwapStates) { a.Lot.Owner = mill },
		"other lot":         func(b, _ *SwapStates) { b.Listing.LotID = states.LotID("tx9") },
		"lot not seller's":  func(b, _ *SwapStates) { b.Lot.Owner = mill },
		"listing edited":    func(_, a *SwapStates) { a.Listing.Ask = 1 },
		"balance ID":        func(b, a *SwapStates) { b.Seller.ID = states.BalanceID(mill); a.Seller.ID = b.Seller.ID },
		"balance owner":     func(b, a *SwapStates) { b.Seller.Owner = mill; a.Seller.Owner = mill },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b, a := goldSwap()
			edit(&b, &a)
			require.Error(t, CheckSwap(refinery, mine, b, a))
		})
	}

	t.Run("buyer is seller", func(t *testing.T) {
		t.Parallel()
		b, a := goldSwap()
		require.Error(t, CheckSwap(mine, mine, b, a))
	})
}

func TestCheckAgreed(t *testing.T) {
	t.Parallel()

	_, after := goldSwap()
	require.NoError(t, CheckAgreed(states.ListingID("tx1"), 90, after))
	require.Error(t, CheckAgreed(states.ListingID("tx2"), 90, after))
	require.Error(t, CheckAgreed(states.ListingID("tx1"), 95, after))
}

func TestCheckKeys(t *testing.T) {
	t.Parallel()

	want := []string{"listing.tx1", "lot.tx0", states.BalanceID(refinery), states.BalanceID(mine)}
	require.NoError(t, CheckKeys(want, want))
	require.Error(t, CheckKeys(want[:3], want))
	require.NoError(t, CheckKeys([]string{want[1], want[0], want[3], want[2]}, want))
	require.Error(t, CheckKeys([]string{want[0], want[0], want[2], want[3]}, want))
	require.Error(t, CheckKeys([]string{want[0], want[1], want[2], states.BalanceID(mill)}, want))
}
