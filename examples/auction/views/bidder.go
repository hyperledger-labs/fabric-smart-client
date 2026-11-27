/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"crypto/rand"
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/examples/auction/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// The views a bidder runs.

// ownBidKey is where a bidder keeps its own bid in its local store.
func ownBidKey(bidID string) (string, error) {
	return kvs.CreateCompositeKey("own-bid", []string{bidID})
}

// PlaceBid is the input of PlaceBidView.
type PlaceBid struct {
	AuctionID string
	Price     uint64
	// House is the FSC node of the auction house.
	House view.Identity
}

// PlaceBidView places a sealed bid and returns its ID. The bidder builds one
// transaction that reads the auction and writes the bid's commitment, signs
// it, and asks the house to endorse it. The house sees the bid in clear, and
// the ledger sees only its hash, in a transaction the bidder signed.
type PlaceBidView struct {
	PlaceBid
}

func (p *PlaceBidView) Call(viewCtx view.Context) (any, error) {
	me, err := myIdentity(viewCtx)
	if err != nil {
		return nil, err
	}

	tx, err := newTransaction(viewCtx, "bid", me)
	if err != nil {
		return nil, err
	}

	// Reading the auction makes the bid depend on its current version: if the
	// house closes the auction first, the bid fails at commit.
	auction := &states.Auction{}
	if err := tx.AddInputByLinearID(p.AuctionID, auction); err != nil {
		return nil, errors.Wrapf(err, "failed reading auction [%s]", p.AuctionID)
	}

	// The random nonce makes the hash on the ledger hide the price: without it,
	// anyone could hash every plausible price and compare.
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, errors.Wrap(err, "failed generating the nonce")
	}
	// The bid takes the transaction's ID, which is unique: the house checks it.
	bid := &states.Bid{ID: tx.ID(), AuctionID: auction.ID, Bidder: me, Price: p.Price, Nonce: nonce}
	// WithHashHiding puts the hash of the bid on the ledger and carries the bid
	// itself to the endorsers in transient data, which is never ordered.
	if err := tx.AddOutput(bid, state.WithHashHiding()); err != nil {
		return nil, errors.Wrap(err, "failed adding the bid")
	}

	// The bidder signs first, then the house checks and endorses.
	if err := submit(viewCtx, tx, me, p.House); err != nil {
		return nil, errors.WithMessage(err, "failed placing the bid")
	}

	// Keep the bid: it is the proof to check the house's reveal against.
	store, err := kvsOf(viewCtx)
	if err != nil {
		return nil, err
	}
	key, err := ownBidKey(bid.ID)
	if err != nil {
		return nil, errors.Wrap(err, "failed building the bid key")
	}
	if err := store.Put(viewCtx.Context(), key, bid); err != nil {
		return nil, errors.Wrapf(err, "failed storing bid [%s]", bid.ID)
	}
	return bid.ID, nil
}

// PlaceBidViewFactory creates a PlaceBidView from a JSON PlaceBid.
type PlaceBidViewFactory struct{}

func (*PlaceBidViewFactory) NewView(in []byte) (view.View, error) {
	f := &PlaceBidView{}
	if err := json.Unmarshal(in, &f.PlaceBid); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}

// AuditBid is the input of AuditView.
type AuditBid struct {
	AuctionID string
	// BidID is the bid this bidder placed.
	BidID string
}

// AuditView checks the house's reveal against the ledger and the bidder's own
// bid, and returns an Outcome. It reads only: no transaction, no other party
// involved.
type AuditView struct {
	AuditBid
}

func (a *AuditView) Call(viewCtx view.Context) (any, error) {
	vault, err := state.GetVault(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the vault")
	}
	auction := &states.Auction{}
	if err := vault.GetState(viewCtx.Context(), Namespace, a.AuctionID, auction); err != nil {
		return nil, errors.Wrapf(err, "failed reading auction [%s]", a.AuctionID)
	}
	if auction.Status != states.Ended {
		return NotEnded, nil
	}

	store, err := kvsOf(viewCtx)
	if err != nil {
		return nil, err
	}
	key, err := ownBidKey(a.BidID)
	if err != nil {
		return nil, errors.Wrap(err, "failed building the bid key")
	}
	own := &states.Bid{}
	if err := store.Get(viewCtx.Context(), key, own); err != nil {
		return nil, errors.Wrapf(err, "unknown bid [%s]", a.BidID)
	}

	revealed := &states.Bid{}
	if err := json.Unmarshal(auction.WinningBid, revealed); err != nil {
		return nil, errors.Wrap(err, "failed decoding the winning bid")
	}
	// Read the commitments of the winning bid and of the bidder's own bid. The
	// state vault decodes JSON states, so read the raw values: for a bid it is
	// the hash.
	_, ch, err := fabric.GetDefaultChannel(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the default channel")
	}
	qe, err := ch.Vault().NewQueryExecutor(viewCtx.Context())
	if err != nil {
		return nil, errors.Wrap(err, "failed opening a vault query")
	}
	defer func() { _ = qe.Done() }()
	winnerCommitted, err := committedValue(viewCtx, qe, revealed.ID)
	if err != nil {
		return nil, err
	}
	ownCommitted, err := committedValue(viewCtx, qe, own.ID)
	if err != nil {
		return nil, err
	}

	return Audit(auction, winnerCommitted, ownCommitted, own)
}

// committedValue returns the value the ledger holds under a bid's key: its hash,
// or nil if there is none.
func committedValue(viewCtx view.Context, qe driver.QueryExecutor, bidID string) ([]byte, error) {
	read, err := qe.GetState(viewCtx.Context(), Namespace, bidID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed reading the commitment of bid [%s]", bidID)
	}
	if read == nil {
		return nil, nil
	}
	return read.Raw, nil
}

// AuditViewFactory creates an AuditView from a JSON AuditBid.
type AuditViewFactory struct{}

func (*AuditViewFactory) NewView(in []byte) (view.View, error) {
	f := &AuditView{}
	if err := json.Unmarshal(in, &f.AuditBid); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}
