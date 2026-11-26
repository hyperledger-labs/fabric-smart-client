/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/examples/trading/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// A trade: the buyer finds a listing on the market's board, negotiates the
// price with the seller in a session between their two nodes, and the two sign
// one swap transaction that the market endorses.

// Buy is the input of BuyView.
type Buy struct {
	// Kind and Qty say what the buyer looks for.
	Kind states.Kind
	Qty  uint64
	// Opening is the buyer's first offer, Budget the most it pays, and Step
	// how far it comes up per counter.
	Opening, Budget, Step uint64
	// Market is the FSC node of the market.
	Market view.Identity
}

// BuyResult is how a trade ended.
type BuyResult struct {
	Deal bool
	// ListingID and Price are set on a deal.
	ListingID string
	Price     uint64
	// Reason says why there was no deal.
	Reason string
}

// BuyView buys a lot: browse, negotiate, swap.
type BuyView struct {
	Buy
}

func (b *BuyView) Call(viewCtx view.Context) (any, error) {
	me, err := myIdentity(viewCtx)
	if err != nil {
		return nil, err
	}

	// 1. Browse. The first open listing from another producer wins.
	res, err := viewCtx.RunView(&BrowseView{Browse{Kind: b.Kind, Qty: b.Qty, Market: b.Market}})
	if err != nil {
		return nil, errors.WithMessage(err, "failed browsing")
	}
	listings, ok := res.([]*states.Listing)
	if !ok {
		return nil, errors.Errorf("unexpected browse result [%T]", res)
	}
	var listing *states.Listing
	for _, l := range listings {
		if !l.Seller.Equal(me) {
			listing = l
			break
		}
	}
	if listing == nil {
		return &BuyResult{Reason: "no open listing"}, nil
	}

	// 2. Negotiate, directly with the seller's node. None of this reaches the
	// ledger, or the market.
	s, err := session.NewJSON(viewCtx, viewCtx.Initiator(), listing.Seller)
	if err != nil {
		return nil, errors.Wrap(err, "failed opening a session with the seller")
	}
	n := &Negotiator{Price: b.Opening, Limit: b.Budget, Step: b.Step}
	if err := s.Send(viewCtx.Context(), &Message{Decision: Offer, ListingID: listing.ID, Price: b.Opening}); err != nil {
		return nil, errors.Wrap(err, "failed sending the offer")
	}
	var price uint64
	for agreed := false; !agreed; {
		answer := &Message{}
		if err := s.ReceiveWithTimeout(answer, time.Minute); err != nil {
			return nil, errors.Wrap(err, "failed receiving the seller's answer")
		}
		switch answer.Decision {
		case Accept:
			// The seller took our last price.
			price, agreed = n.Price, true
		case Counter:
			reply := n.Respond(answer.Price)
			if err := s.Send(viewCtx.Context(), &reply); err != nil {
				return nil, errors.Wrap(err, "failed answering the seller")
			}
			switch reply.Decision {
			case Accept:
				price, agreed = reply.Price, true
			case Reject:
				return &BuyResult{Reason: reply.Reason}, nil
			}
		case Reject:
			return &BuyResult{Reason: answer.Reason}, nil
		default:
			return nil, errors.Errorf("unexpected answer [%s]", answer.Decision)
		}
	}

	// 3. Swap: the lot to the buyer, the coins to the seller, the listing sold,
	// in one transaction. The buyer signs, then the seller, over the same
	// session as the negotiation, then the market endorses.
	tx, err := newTransaction(viewCtx, "swap", me, listing.Seller)
	if err != nil {
		return nil, err
	}
	sold, lot, buyer, seller := &states.Listing{}, &states.Lot{}, &states.Balance{}, &states.Balance{}
	for _, in := range []struct {
		key string
		st  any
	}{
		{listing.ID, sold},
		{listing.LotID, lot},
		{states.BalanceID(me), buyer},
		{states.BalanceID(listing.Seller), seller},
	} {
		if err := tx.AddInputByLinearID(in.key, in.st); err != nil {
			return nil, errors.Wrapf(err, "failed reading [%s]", in.key)
		}
	}
	if buyer.Coins < price {
		return nil, errors.Errorf("cannot pay %d with %d coins", price, buyer.Coins)
	}
	sold.Status, sold.Price = states.Sold, price
	lot.Owner = me
	buyer.Coins -= price
	seller.Coins += price
	for _, out := range []any{sold, lot, buyer, seller} {
		if err := tx.AddOutput(out); err != nil {
			return nil, errors.Wrap(err, "failed adding the swap's outputs")
		}
	}
	if err := submit(viewCtx, tx, me, listing.Seller, b.Market); err != nil {
		return nil, errors.WithMessage(err, "failed swapping")
	}
	return &BuyResult{Deal: true, ListingID: listing.ID, Price: price}, nil
}

// BuyViewFactory creates a BuyView from a JSON Buy.
type BuyViewFactory struct{}

func (*BuyViewFactory) NewView(in []byte) (view.View, error) {
	f := &BuyView{}
	if err := json.Unmarshal(in, &f.Buy); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}

// held are the listings this seller is negotiating. A listing is held by one
// buyer at a time, so the seller never agrees, or signs, two swaps of the same
// lot.
// ponytail: in-memory hold, lost on restart; the ledger still prevents a double sale.
var held sync.Map

// hold reports whether listingID was free and is now held.
func hold(listingID string) bool {
	_, taken := held.LoadOrStore(listingID, struct{}{})
	return !taken
}

func release(listingID string) { held.Delete(listingID) }

// SellerView is the seller's side of BuyView: it negotiates with the buyer,
// then checks and signs the swap.
type SellerView struct{}

func (*SellerView) Call(viewCtx view.Context) (any, error) {
	me, err := myIdentity(viewCtx)
	if err != nil {
		return nil, err
	}
	s := session.JSON(viewCtx)
	offer := &Message{}
	if err := s.ReceiveWithTimeout(offer, time.Minute); err != nil {
		return nil, errors.Wrap(err, "failed receiving the offer")
	}
	if offer.Decision != Offer {
		return nil, errors.Errorf("expected an offer, got [%s]", offer.Decision)
	}
	reject := func(reason string) error {
		return s.Send(viewCtx.Context(), &Message{Decision: Reject, Reason: reason})
	}

	listing := &states.Listing{}
	if err := getState(viewCtx, offer.ListingID, listing); err != nil || !forSale(viewCtx, listing) {
		return nil, reject("not open")
	}
	if !hold(listing.ID) {
		return nil, reject("busy")
	}
	defer release(listing.ID)

	store, err := kvsOf(viewCtx)
	if err != nil {
		return nil, err
	}
	key, err := termsKey(listing.ID)
	if err != nil {
		return nil, errors.Wrap(err, "failed building the terms key")
	}
	terms := &Terms{}
	if err := store.Get(viewCtx.Context(), key, terms); err != nil {
		return nil, errors.Wrapf(err, "unknown listing [%s]", listing.ID)
	}

	// Negotiate until one side accepts or rejects.
	n := &Negotiator{Selling: true, Price: listing.Ask, Limit: terms.Floor, Step: terms.Step}
	theirs := offer.Price
	var price uint64
	for agreed := false; !agreed; {
		reply := n.Respond(theirs)
		if err := s.Send(viewCtx.Context(), &reply); err != nil {
			return nil, errors.Wrap(err, "failed answering the buyer")
		}
		switch reply.Decision {
		case Accept:
			price, agreed = reply.Price, true
			continue
		case Reject:
			return nil, nil
		}
		answer := &Message{}
		if err := s.ReceiveWithTimeout(answer, time.Minute); err != nil {
			return nil, errors.Wrap(err, "failed receiving the buyer's answer")
		}
		switch answer.Decision {
		case Accept:
			// The buyer took our last counter.
			price, agreed = n.Price, true
		case Counter:
			theirs = answer.Price
		case Reject:
			return nil, nil
		default:
			return nil, errors.Errorf("unexpected answer [%s]", answer.Decision)
		}
	}

	// The buyer builds the swap. Sign only what was agreed.
	tx, err := state.ReceiveTransaction(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed receiving the swap")
	}
	sw, err := readSwap(tx)
	if err != nil {
		return nil, err
	}
	if !sw.seller.Equal(me) {
		return nil, errors.New("the swap names another seller")
	}
	if err := errors.Join(
		CheckSwap(sw.buyer, me, sw.before, sw.after),
		CheckAgreed(listing.ID, price, sw.after),
	); err != nil {
		return nil, errors.WithMessagef(err, "refusing to sign swap [%s]", tx.ID())
	}
	if _, err := viewCtx.RunView(state.NewEndorseView(tx)); err != nil {
		return nil, errors.WithMessagef(err, "failed signing swap [%s]", tx.ID())
	}
	if _, err := viewCtx.RunView(state.NewFinalityWithTimeoutView(tx, time.Minute)); err != nil {
		return nil, errors.WithMessagef(err, "swap [%s] did not commit", tx.ID())
	}
	return nil, nil
}

// swap is a swap transaction, decoded.
type swap struct {
	buyer, seller view.Identity
	before, after SwapStates
}

// readSwap decodes a swap transaction and checks it touches exactly the
// listing, its lot, and the two parties' balances. The states
// before come from the ledger, not from the transaction's author.
func readSwap(tx *state.Transaction) (*swap, error) {
	commands, err := tx.Commands()
	if err != nil {
		return nil, errors.Wrap(err, "failed reading the commands")
	}
	if commands.Count() != 1 || commands.At(0).Name != "swap" || len(commands.At(0).Ids) != 2 {
		return nil, errors.New("expected one command [swap] with buyer and seller")
	}
	sw := &swap{buyer: commands.At(0).Ids[0], seller: commands.At(0).Ids[1]}

	c, err := changesOf(tx)
	if err != nil {
		return nil, err
	}
	var listingKey string
	for _, k := range c.inputs {
		if strings.HasPrefix(k, states.ListingPrefix) {
			listingKey = k
		}
	}
	listing := &states.Listing{}
	if err := inputByKey(tx, listingKey, listing); err != nil {
		return nil, err
	}
	want := []string{listingKey, listing.LotID, states.BalanceID(sw.buyer), states.BalanceID(sw.seller)}
	if err := errors.Join(
		CheckKeys(c.inputs, want),
		CheckKeys(c.deleted, nil),
		CheckKeys(c.written, want),
	); err != nil {
		return nil, err
	}

	b := SwapStates{Listing: listing, Lot: &states.Lot{}, Buyer: &states.Balance{}, Seller: &states.Balance{}}
	a := SwapStates{Listing: &states.Listing{}, Lot: &states.Lot{}, Buyer: &states.Balance{}, Seller: &states.Balance{}}
	if err := errors.Join(
		inputByKey(tx, want[1], b.Lot),
		inputByKey(tx, want[2], b.Buyer),
		inputByKey(tx, want[3], b.Seller),
		outputByKey(tx, want[0], a.Listing),
		outputByKey(tx, want[1], a.Lot),
		outputByKey(tx, want[2], a.Buyer),
		outputByKey(tx, want[3], a.Seller),
	); err != nil {
		return nil, err
	}
	sw.before, sw.after = b, a
	return sw, nil
}
