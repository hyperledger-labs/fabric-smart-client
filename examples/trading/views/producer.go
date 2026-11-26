/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/examples/trading/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// The views a producer runs without negotiating: each builds a transaction
// the producer signs and the market endorses.

// WhoAmIView returns this node's Fabric identity, which the market registers
// at setup.
type WhoAmIView struct{}

func (*WhoAmIView) Call(viewCtx view.Context) (any, error) {
	return myIdentity(viewCtx)
}

// WhoAmIViewFactory creates a WhoAmIView; it takes no input.
type WhoAmIViewFactory struct{}

func (*WhoAmIViewFactory) NewView([]byte) (view.View, error) { return &WhoAmIView{}, nil }

// Produce is the input of ProduceView.
type Produce struct {
	Kind states.Kind
	Qty  uint64
	// Market is the FSC node of the market, which endorses the transaction.
	Market view.Identity
}

// ProduceView creates a lot of raw goods owned by this producer and returns
// its ID. The producer signs; the market checks the producer may produce this
// kind, and endorses.
type ProduceView struct {
	Produce
}

func (p *ProduceView) Call(viewCtx view.Context) (any, error) {
	me, err := myIdentity(viewCtx)
	if err != nil {
		return nil, err
	}
	tx, err := newTransaction(viewCtx, "produce", me)
	if err != nil {
		return nil, err
	}
	lot := &states.Lot{ID: states.LotID(tx.ID()), Kind: p.Kind, Qty: p.Qty, Owner: me}
	if err := tx.AddOutput(lot); err != nil {
		return nil, errors.Wrap(err, "failed adding the lot")
	}
	if err := submit(viewCtx, tx, me, p.Market); err != nil {
		return nil, errors.WithMessage(err, "failed producing")
	}
	return lot.ID, nil
}

// ProduceViewFactory creates a ProduceView from a JSON Produce.
type ProduceViewFactory struct{}

func (*ProduceViewFactory) NewView(in []byte) (view.View, error) {
	f := &ProduceView{}
	if err := json.Unmarshal(in, &f.Produce); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}

// Refine is the input of RefineView.
type Refine struct {
	// LotID is the raw lot to refine; this producer must own it.
	LotID  string
	Market view.Identity
}

// RefineView consumes a raw lot and creates the refined lot, at 2 raw units
// per refined unit, and returns the new lot's ID.
type RefineView struct {
	Refine
}

func (r *RefineView) Call(viewCtx view.Context) (any, error) {
	me, err := myIdentity(viewCtx)
	if err != nil {
		return nil, err
	}
	tx, err := newTransaction(viewCtx, "refine", me)
	if err != nil {
		return nil, err
	}

	raw := &states.Lot{}
	if err := tx.AddInputByLinearID(r.LotID, raw); err != nil {
		return nil, errors.Wrapf(err, "failed reading lot [%s]", r.LotID)
	}
	// Deleting the raw lot and creating the refined one happen in the same
	// transaction: the goods cannot be refined twice.
	if err := tx.Delete(raw); err != nil {
		return nil, errors.Wrapf(err, "failed consuming lot [%s]", r.LotID)
	}
	// The producer proposes the refined lot; the market's registry decides
	// whether this producer may refine this kind at all.
	to := map[states.Kind]states.Kind{states.Gold: states.Bar, states.Wood: states.Plank}[raw.Kind]
	refined := &states.Lot{ID: states.LotID(tx.ID()), Kind: to, Qty: raw.Qty / 2, Owner: me}
	if err := tx.AddOutput(refined); err != nil {
		return nil, errors.Wrap(err, "failed adding the refined lot")
	}
	if err := submit(viewCtx, tx, me, r.Market); err != nil {
		return nil, errors.WithMessage(err, "failed refining")
	}
	return refined.ID, nil
}

// RefineViewFactory creates a RefineView from a JSON Refine.
type RefineViewFactory struct{}

func (*RefineViewFactory) NewView(in []byte) (view.View, error) {
	f := &RefineView{}
	if err := json.Unmarshal(in, &f.Refine); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}

// List is the input of ListView.
type List struct {
	LotID string
	// Ask is the public asking price.
	Ask uint64
	// Floor and Step are the seller's private terms: the lowest price it
	// accepts, and how far it comes down per counter.
	Floor, Step uint64
	Market      view.Identity
}

// Terms are what a seller keeps to itself about one of its listings.
type Terms struct {
	Floor, Step uint64
}

// termsKey is where a seller keeps the terms of a listing in its local store.
func termsKey(listingID string) (string, error) {
	return kvs.CreateCompositeKey("terms", []string{listingID})
}

// ListView offers a lot this producer owns on the market and returns the
// listing's ID. Only the ask goes on the ledger; the floor stays on this node.
type ListView struct {
	List
}

func (l *ListView) Call(viewCtx view.Context) (any, error) {
	me, err := myIdentity(viewCtx)
	if err != nil {
		return nil, err
	}
	tx, err := newTransaction(viewCtx, "list", me)
	if err != nil {
		return nil, err
	}

	// Reading the lot ties the listing to the lot's current owner.
	lot := &states.Lot{}
	if err := tx.AddInputByLinearID(l.LotID, lot); err != nil {
		return nil, errors.Wrapf(err, "failed reading lot [%s]", l.LotID)
	}
	listing := &states.Listing{
		ID: states.ListingID(tx.ID()), LotID: lot.ID, Kind: lot.Kind, Qty: lot.Qty,
		Seller: me, Ask: l.Ask, Status: states.Open,
	}
	if err := tx.AddOutput(listing); err != nil {
		return nil, errors.Wrap(err, "failed adding the listing")
	}
	if err := submit(viewCtx, tx, me, l.Market); err != nil {
		return nil, errors.WithMessage(err, "failed listing")
	}

	store, err := kvsOf(viewCtx)
	if err != nil {
		return nil, err
	}
	key, err := termsKey(listing.ID)
	if err != nil {
		return nil, errors.Wrap(err, "failed building the terms key")
	}
	if err := store.Put(viewCtx.Context(), key, &Terms{Floor: l.Floor, Step: l.Step}); err != nil {
		return nil, errors.Wrapf(err, "failed storing the terms of listing [%s]", listing.ID)
	}
	return listing.ID, nil
}

// ListViewFactory creates a ListView from a JSON List.
type ListViewFactory struct{}

func (*ListViewFactory) NewView(in []byte) (view.View, error) {
	f := &ListView{}
	if err := json.Unmarshal(in, &f.List); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}
