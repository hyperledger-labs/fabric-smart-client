/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/examples/auction/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Namespace is where the auction's states live on the ledger.
const Namespace = "auction"

// newTransaction starts a transaction in the auction namespace with a single
// command and its party.
func newTransaction(viewCtx view.Context, command string, party view.Identity) (*state.Transaction, error) {
	tx, err := state.NewTransaction(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating a new transaction")
	}
	tx.SetNamespace(Namespace)
	if err := tx.AddCommand(command, party); err != nil {
		return nil, errors.Wrapf(err, "failed adding command [%s]", command)
	}
	return tx, nil
}

// submit collects the endorsements of parties, in order, then sends the
// transaction for ordering and waits until it is committed. A party that is
// this node endorses locally, so the house submits its own transactions with
// itself as the only party.
func submit(viewCtx view.Context, tx *state.Transaction, parties ...view.Identity) error {
	if _, err := viewCtx.RunView(state.NewCollectEndorsementsView(tx, parties...)); err != nil {
		return errors.WithMessage(err, "failed collecting endorsements")
	}
	if _, err := viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, time.Minute)); err != nil {
		return errors.WithMessage(err, "failed ordering the transaction")
	}
	return nil
}

// myIdentity returns this node's Fabric identity: the house endorses with it,
// a bidder signs its bids with it.
func myIdentity(viewCtx view.Context) (view.Identity, error) {
	fns, _, err := fabric.GetDefaultChannel(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the default network")
	}
	return fns.IdentityProvider().DefaultIdentity(), nil
}

// kvsOf returns this node's local key-value store. What is stored there never
// leaves the node.
func kvsOf(viewCtx view.Context) (*kvs.KVS, error) {
	s, err := viewCtx.GetService(reflect.TypeFor[*kvs.KVS]())
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the local store")
	}
	store, ok := s.(*kvs.KVS)
	if !ok {
		return nil, errors.Errorf("unexpected local store type [%T]", s)
	}
	return store, nil
}

// QueryAuction is the input of QueryAuctionView.
type QueryAuction struct {
	AuctionID string
}

// QueryAuctionView returns the auction as committed on the ledger. Any node can
// run it: the auction is stored in clear.
type QueryAuctionView struct {
	QueryAuction
}

func (q *QueryAuctionView) Call(viewCtx view.Context) (any, error) {
	vault, err := state.GetVault(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the vault")
	}
	auction := &states.Auction{}
	if err := vault.GetState(viewCtx.Context(), Namespace, q.AuctionID, auction); err != nil {
		return nil, errors.Wrapf(err, "failed reading auction [%s]", q.AuctionID)
	}
	return auction, nil
}

// QueryAuctionViewFactory creates a QueryAuctionView from a JSON QueryAuction.
type QueryAuctionViewFactory struct{}

func (*QueryAuctionViewFactory) NewView(in []byte) (view.View, error) {
	f := &QueryAuctionView{}
	if err := json.Unmarshal(in, &f.QueryAuction); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}
