/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/examples/auction/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// The views the auction house runs.

// InitView makes the house process the auction namespace: as its endorser, it
// tracks the transactions committed there.
type InitView struct{}

func (*InitView) Call(viewCtx view.Context) (any, error) {
	_, ch, err := fabric.GetDefaultChannel(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the default channel")
	}
	if err := ch.Committer().ProcessNamespace(Namespace); err != nil {
		return nil, errors.Wrapf(err, "failed processing namespace [%s]", Namespace)
	}
	return nil, nil
}

// InitViewFactory creates an InitView; it takes no input.
type InitViewFactory struct{}

func (*InitViewFactory) NewView([]byte) (view.View, error) { return &InitView{}, nil }

// CreateAuction is the input of CreateAuctionView.
type CreateAuction struct {
	// Item is what is being auctioned.
	Item string
}

// CreateAuctionView opens an auction and returns its ID. The house is the only
// party: it builds the transaction, endorses it itself, and submits it.
type CreateAuctionView struct {
	CreateAuction
}

func (c *CreateAuctionView) Call(viewCtx view.Context) (any, error) {
	house, err := myIdentity(viewCtx)
	if err != nil {
		return nil, err
	}

	tx, err := newTransaction(viewCtx, "create", house)
	if err != nil {
		return nil, err
	}

	// AddOutput assigns the auction its ID.
	auction := &states.Auction{Item: c.Item, Status: states.Open}
	if err := tx.AddOutput(auction); err != nil {
		return nil, errors.Wrap(err, "failed adding the auction")
	}

	if err := submit(viewCtx, tx, house); err != nil {
		return nil, errors.WithMessage(err, "failed creating the auction")
	}
	return auction.ID, nil
}

// CreateAuctionViewFactory creates a CreateAuctionView from a JSON
// CreateAuction.
type CreateAuctionViewFactory struct{}

func (*CreateAuctionViewFactory) NewView(in []byte) (view.View, error) {
	f := &CreateAuctionView{}
	if err := json.Unmarshal(in, &f.CreateAuction); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}

// RecordedBid is a bid the house endorsed, with the transaction that commits it.
type RecordedBid struct {
	TxID string
	Bid  states.Bid
}

// recordedBidKey is where the house keeps a bid it endorsed, grouped by auction.
func recordedBidKey(auctionID, bidID string) (string, error) {
	return kvs.CreateCompositeKey("auction-bid", []string{auctionID, bidID})
}

// BidResponderView is the house's side of PlaceBidView. It plays the role a
// chaincode plays in Fabric: it validates the transaction and endorses it, but
// it is ordinary Go code running on the house's own node. An error returned
// here reaches the bidder, and the house does not endorse.
type BidResponderView struct{}

func (*BidResponderView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.ReceiveTransaction(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed receiving the bid")
	}

	bid, err := validateBid(tx)
	if err != nil {
		return nil, errors.WithMessagef(err, "rejected bid in transaction [%s]", tx.ID())
	}

	// Remember the bid before endorsing it. Reveal keeps only the bids whose
	// transaction is committed as valid.
	store, err := kvsOf(viewCtx)
	if err != nil {
		return nil, err
	}
	key, err := recordedBidKey(bid.AuctionID, bid.ID)
	if err != nil {
		return nil, errors.Wrap(err, "failed building the bid key")
	}
	if err := store.Put(viewCtx.Context(), key, &RecordedBid{TxID: tx.ID(), Bid: *bid}); err != nil {
		return nil, errors.Wrapf(err, "failed recording bid [%s]", bid.ID)
	}

	if _, err := viewCtx.RunView(state.NewEndorseView(tx)); err != nil {
		return nil, errors.WithMessagef(err, "failed endorsing bid [%s]", bid.ID)
	}
	if _, err := viewCtx.RunView(state.NewFinalityWithTimeoutView(tx, time.Minute)); err != nil {
		return nil, errors.WithMessagef(err, "bid [%s] did not commit", bid.ID)
	}
	return nil, nil
}

// validateBid checks a bid transaction the way a chaincode would, and returns
// the bid in clear.
func validateBid(tx *state.Transaction) (*states.Bid, error) {
	namespaces, err := tx.Namespaces()
	if err != nil {
		return nil, errors.Wrap(err, "failed reading the namespaces")
	}
	if len(namespaces) != 1 || namespaces[0] != Namespace {
		return nil, errors.Errorf("expected only namespace [%s], got %v", Namespace, namespaces)
	}

	commands, err := tx.Commands()
	if err != nil {
		return nil, errors.Wrap(err, "failed reading the commands")
	}
	if commands.Count() != 1 {
		return nil, errors.Errorf("expected a single command, got %d", commands.Count())
	}
	command := commands.At(0)
	if command.Name != "bid" || len(command.Ids) != 1 {
		return nil, errors.Errorf("expected command [bid] with the bidder as the only party")
	}

	numInputs, err := tx.NumInputs()
	if err != nil {
		return nil, errors.Wrap(err, "failed reading the inputs")
	}
	numOutputs, err := tx.NumOutputs()
	if err != nil {
		return nil, errors.Wrap(err, "failed reading the outputs")
	}
	if numInputs != 1 || numOutputs != 1 {
		return nil, errors.Errorf("expected the auction as the only input and the bid as the only output, got %d inputs and %d outputs", numInputs, numOutputs)
	}

	// The auction must still be open. The house reads it from the ledger, not
	// from anything the bidder sent.
	auction := &states.Auction{}
	if err := tx.GetInputAt(0, auction); err != nil {
		return nil, errors.Wrap(err, "failed reading the auction")
	}
	if auction.Status != states.Open {
		return nil, errors.Errorf("auction [%s] is not open", auction.ID)
	}

	// The bid in clear comes from transient data; the ledger gets its hash.
	bid := &states.Bid{}
	if err := tx.GetOutputAt(0, bid); err != nil {
		return nil, errors.Wrap(err, "failed reading the bid")
	}
	rws, err := tx.Namespace.RWSet()
	if err != nil {
		return nil, errors.Wrap(err, "failed reading the read-write set")
	}
	key, written, err := rws.GetWriteAt(Namespace, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed reading the bid's write")
	}
	// The bid's key is its transaction's ID, so it cannot overwrite the auction
	// or another bid.
	if err := CheckBidKey(key, tx.ID(), bid); err != nil {
		return nil, err
	}
	// Nothing else ties the bid to the hash: check it, or a bidder could commit
	// one price and show the house another.
	if err := CheckCommitment(written, bid); err != nil {
		return nil, err
	}

	if bid.AuctionID != auction.ID {
		return nil, errors.Errorf("bid [%s] is for another auction", bid.ID)
	}
	if !bid.Bidder.Equal(command.Ids[0]) {
		return nil, errors.Errorf("bid [%s] does not belong to the command's party", bid.ID)
	}
	if err := tx.HasBeenEndorsedBy(bid.Bidder); err != nil {
		return nil, errors.WithMessagef(err, "bid [%s] is not signed by its bidder", bid.ID)
	}
	if bid.Price == 0 {
		return nil, errors.Errorf("bid [%s] has no price", bid.ID)
	}
	return bid, nil
}

// CloseAuction is the input of CloseAuctionView.
type CloseAuction struct {
	AuctionID string
}

// CloseAuctionView stops the auction from accepting bids. Bids still in flight
// read the open auction's old version, so they fail at commit.
type CloseAuctionView struct {
	CloseAuction
}

func (c *CloseAuctionView) Call(viewCtx view.Context) (any, error) {
	house, err := myIdentity(viewCtx)
	if err != nil {
		return nil, err
	}

	tx, err := newTransaction(viewCtx, "close", house)
	if err != nil {
		return nil, err
	}

	auction := &states.Auction{}
	if err := tx.AddInputByLinearID(c.AuctionID, auction); err != nil {
		return nil, errors.Wrapf(err, "failed reading auction [%s]", c.AuctionID)
	}
	if auction.Status != states.Open {
		return nil, errors.Errorf("auction [%s] is not open", auction.ID)
	}
	auction.Status = states.Closed
	if err := tx.AddOutput(auction); err != nil {
		return nil, errors.Wrap(err, "failed adding the auction")
	}

	if err := submit(viewCtx, tx, house); err != nil {
		return nil, errors.WithMessagef(err, "failed closing auction [%s]", auction.ID)
	}
	return nil, nil
}

// CloseAuctionViewFactory creates a CloseAuctionView from a JSON CloseAuction.
type CloseAuctionViewFactory struct{}

func (*CloseAuctionViewFactory) NewView(in []byte) (view.View, error) {
	f := &CloseAuctionView{}
	if err := json.Unmarshal(in, &f.CloseAuction); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}

// Reveal is the input of RevealView.
type Reveal struct {
	AuctionID string
}

// RevealView ends a closed auction and returns the winning bid. Among the bids
// the house endorsed whose transaction is valid, it picks the winner and
// publishes it in clear. Anyone can hash the published bid and compare it with
// the commitment on the ledger.
type RevealView struct {
	Reveal
}

func (r *RevealView) Call(viewCtx view.Context) (any, error) {
	house, err := myIdentity(viewCtx)
	if err != nil {
		return nil, err
	}

	tx, err := newTransaction(viewCtx, "reveal", house)
	if err != nil {
		return nil, err
	}

	auction := &states.Auction{}
	if err := tx.AddInputByLinearID(r.AuctionID, auction); err != nil {
		return nil, errors.Wrapf(err, "failed reading auction [%s]", r.AuctionID)
	}
	// Once the close is committed, every bid transaction is final.
	if auction.Status != states.Closed {
		return nil, errors.Errorf("auction [%s] is not closed", auction.ID)
	}

	bids, err := validBids(viewCtx, auction.ID)
	if err != nil {
		return nil, err
	}
	winner := Winner(bids)
	if winner == nil {
		return nil, errors.Errorf("auction [%s] has no valid bids", auction.ID)
	}

	// Publish the winning bid's preimage: the exact bytes its commitment hashed.
	raw, err := json.Marshal(winner)
	if err != nil {
		return nil, errors.Wrap(err, "failed encoding the winning bid")
	}
	auction.Status = states.Ended
	auction.Winner = winner.Bidder
	auction.WinningBid = raw
	if err := tx.AddOutput(auction); err != nil {
		return nil, errors.Wrap(err, "failed adding the auction")
	}

	if err := submit(viewCtx, tx, house); err != nil {
		return nil, errors.WithMessagef(err, "failed revealing the winner of auction [%s]", auction.ID)
	}
	return winner, nil
}

// validBids returns the bids the house endorsed for the auction whose
// transaction committed as valid. A bid the house endorsed may still have
// failed to commit, for example by racing the close.
func validBids(viewCtx view.Context, auctionID string) ([]*states.Bid, error) {
	store, err := kvsOf(viewCtx)
	if err != nil {
		return nil, err
	}
	it, err := store.GetByPartialCompositeID(viewCtx.Context(), "auction-bid", []string{auctionID})
	if err != nil {
		return nil, errors.Wrapf(err, "failed listing the bids of auction [%s]", auctionID)
	}
	defer func() { _ = it.Close() }()

	_, ch, err := fabric.GetDefaultChannel(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the default channel")
	}
	var bids []*states.Bid
	for it.HasNext() {
		rec := &RecordedBid{}
		if _, err := it.Next(rec); err != nil {
			return nil, errors.Wrap(err, "failed reading a recorded bid")
		}
		code, _, err := ch.Vault().Status(viewCtx.Context(), rec.TxID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed reading the status of transaction [%s]", rec.TxID)
		}
		if code == driver.Valid {
			bids = append(bids, &rec.Bid)
		}
	}
	return bids, nil
}

// RevealViewFactory creates a RevealView from a JSON Reveal.
type RevealViewFactory struct{}

func (*RevealViewFactory) NewView(in []byte) (view.View, error) {
	f := &RevealView{}
	if err := json.Unmarshal(in, &f.Reveal); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}
