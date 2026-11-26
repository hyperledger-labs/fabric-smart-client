/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"slices"

	"github.com/hyperledger-labs/fabric-smart-client/examples/trading/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// The market's rules. They are plain functions, so the market and the
// sellers apply the same rules and they are unit tested without a network.

// Produces returns the raw kind role may produce.
func Produces(role states.Role) (states.Kind, bool) {
	switch role {
	case states.GoldProducer:
		return states.Gold, true
	case states.WoodProducer:
		return states.Wood, true
	default:
		return "", false
	}
}

// Refines returns the raw kind role may refine and what it refines it into.
func Refines(role states.Role) (from, to states.Kind, ok bool) {
	switch role {
	case states.GoldRefiner:
		return states.Gold, states.Bar, true
	case states.WoodRefiner:
		return states.Wood, states.Plank, true
	default:
		return "", "", false
	}
}

// CheckProduce returns an error unless producer, with role, may create lot in
// transaction txID.
func CheckProduce(role states.Role, producer view.Identity, txID string, lot *states.Lot) error {
	kind, ok := Produces(role)
	if !ok {
		return errors.Errorf("role [%s] produces nothing", role)
	}
	if lot.ID != states.LotID(txID) {
		return errors.Errorf("lot [%s] must be keyed by its transaction [%s]", lot.ID, txID)
	}
	if lot.Kind != kind {
		return errors.Errorf("role [%s] produces %s, not %s", role, kind, lot.Kind)
	}
	if lot.Qty == 0 {
		return errors.Errorf("lot [%s] is empty", lot.ID)
	}
	if !lot.Owner.Equal(producer) {
		return errors.Errorf("lot [%s] must belong to its producer", lot.ID)
	}
	return nil
}

// CheckRefine returns an error unless producer, with role, may turn raw into
// refined in transaction txID: 2 raw units make 1 refined unit.
func CheckRefine(role states.Role, producer view.Identity, txID string, raw, refined *states.Lot) error {
	from, to, ok := Refines(role)
	if !ok {
		return errors.Errorf("role [%s] refines nothing", role)
	}
	if !raw.Owner.Equal(producer) {
		return errors.Errorf("lot [%s] does not belong to the refiner", raw.ID)
	}
	if raw.Kind != from {
		return errors.Errorf("role [%s] refines %s, not %s", role, from, raw.Kind)
	}
	if raw.Qty == 0 || raw.Qty%2 != 0 {
		return errors.Errorf("lot [%s] must hold an even, non-zero quantity, has %d", raw.ID, raw.Qty)
	}
	want := states.Lot{ID: states.LotID(txID), Kind: to, Qty: raw.Qty / 2, Owner: producer}
	if !sameLot(refined, &want) {
		return errors.Errorf("refining lot [%s] must create %d %s owned by the refiner under [%s]", raw.ID, want.Qty, to, want.ID)
	}
	return nil
}

// CheckList returns an error unless seller may offer lot with listing in
// transaction txID.
func CheckList(seller view.Identity, txID string, lot *states.Lot, listing *states.Listing) error {
	if listing.ID != states.ListingID(txID) {
		return errors.Errorf("listing [%s] must be keyed by its transaction [%s]", listing.ID, txID)
	}
	if !lot.Owner.Equal(seller) || !listing.Seller.Equal(seller) {
		return errors.Errorf("listing [%s]: only the lot's owner may list it", listing.ID)
	}
	if listing.LotID != lot.ID || listing.Kind != lot.Kind || listing.Qty != lot.Qty {
		return errors.Errorf("listing [%s] does not describe lot [%s]", listing.ID, lot.ID)
	}
	if listing.Ask == 0 {
		return errors.Errorf("listing [%s] has no asking price", listing.ID)
	}
	if listing.Status != states.Open || listing.Price != 0 {
		return errors.Errorf("listing [%s] must start open, without a price", listing.ID)
	}
	return nil
}

// SwapStates are the four states a swap reads and writes.
type SwapStates struct {
	Listing *states.Listing
	Lot     *states.Lot
	Buyer   *states.Balance
	Seller  *states.Balance
}

// CheckSwap returns an error unless after is before with the listed lot sold
// to buyer: the listing is sold at after.Listing.Price, the lot belongs to
// buyer, and that price has moved from buyer's balance to seller's. Nothing
// else may change.
func CheckSwap(buyer, seller view.Identity, before, after SwapStates) error {
	listing, lot := before.Listing, before.Lot
	if buyer.Equal(seller) {
		return errors.Errorf("listing [%s]: buyer and seller are the same", listing.ID)
	}
	if listing.Status != states.Open {
		return errors.Errorf("listing [%s] is not open", listing.ID)
	}
	if !listing.Seller.Equal(seller) || !lot.Owner.Equal(seller) || listing.LotID != lot.ID {
		return errors.Errorf("listing [%s] is not the seller's offer of lot [%s]", listing.ID, lot.ID)
	}
	if err := checkBalance(before.Buyer, buyer); err != nil {
		return err
	}
	if err := checkBalance(before.Seller, seller); err != nil {
		return err
	}

	price := after.Listing.Price
	if price == 0 {
		return errors.Errorf("listing [%s] sold without a price", listing.ID)
	}
	if before.Buyer.Coins < price {
		return errors.Errorf("buyer has %d coins, cannot pay %d", before.Buyer.Coins, price)
	}
	sold := *listing
	sold.Status, sold.Price = states.Sold, price
	if !sameListing(after.Listing, &sold) {
		return errors.Errorf("listing [%s] may only change to sold at the agreed price", listing.ID)
	}
	bought := *lot
	bought.Owner = buyer
	if !sameLot(after.Lot, &bought) {
		return errors.Errorf("lot [%s] may only change owner, to the buyer", lot.ID)
	}
	if !sameBalance(after.Buyer, before.Buyer, before.Buyer.Coins-price) {
		return errors.Errorf("buyer must pay exactly %d", price)
	}
	if !sameBalance(after.Seller, before.Seller, before.Seller.Coins+price) {
		return errors.Errorf("seller must receive exactly %d", price)
	}
	return nil
}

// CheckAgreed returns an error unless the swap sells listingID at price: what
// the seller agreed to in the negotiation.
func CheckAgreed(listingID string, price uint64, after SwapStates) error {
	if after.Listing.ID != listingID || after.Listing.Price != price {
		return errors.Errorf("swap sells listing [%s] at %d, agreed was [%s] at %d", after.Listing.ID, after.Listing.Price, listingID, price)
	}
	return nil
}

// CheckKeys returns an error unless a transaction touches exactly the keys
// want, in any order (a read-write set keeps its keys sorted). The rules above
// check states; this ties those states to the keys the transaction touches.
func CheckKeys(got, want []string) error {
	if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))) {
		return errors.Errorf("transaction touches keys %v, expected %v", got, want)
	}
	return nil
}

// checkBalance returns an error unless b is owner's balance.
func checkBalance(b *states.Balance, owner view.Identity) error {
	if b.ID != states.BalanceID(owner) || !b.Owner.Equal(owner) {
		return errors.Errorf("balance [%s] does not belong to the expected party", b.ID)
	}
	return nil
}

func sameLot(a, b *states.Lot) bool {
	return a.ID == b.ID && a.Kind == b.Kind && a.Qty == b.Qty && a.Owner.Equal(b.Owner)
}

func sameListing(a, b *states.Listing) bool {
	return a.ID == b.ID && a.LotID == b.LotID && a.Kind == b.Kind && a.Qty == b.Qty &&
		a.Seller.Equal(b.Seller) && a.Ask == b.Ask && a.Status == b.Status && a.Price == b.Price
}

func sameBalance(after, before *states.Balance, coins uint64) bool {
	return after.ID == before.ID && after.Owner.Equal(before.Owner) && after.Coins == coins
}
