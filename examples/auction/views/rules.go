/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/examples/auction/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// The auction rules. They are plain functions, so every party applies the
// same rules and they are unit tested without a network.

// Commitment returns the value state.WithHashHiding writes on the ledger for
// bid: the SHA-256 digest of its JSON encoding.
func Commitment(bid *states.Bid) ([]byte, error) {
	raw, err := json.Marshal(bid)
	if err != nil {
		return nil, errors.Wrap(err, "failed encoding bid")
	}
	digest := sha256.Sum256(raw)
	return digest[:], nil
}

// CheckCommitment returns an error unless written, the value on the ledger, is
// the commitment of bid. The state service does not check that the preimage
// carried in transient data matches the digest it writes, so the house checks
// it before endorsing a bid.
func CheckCommitment(written []byte, bid *states.Bid) error {
	expected, err := Commitment(bid)
	if err != nil {
		return err
	}
	if !bytes.Equal(written, expected) {
		return errors.Errorf("bid [%s] does not match its commitment", bid.ID)
	}
	return nil
}

// Winner returns the highest bid; between equal prices the smallest bid ID
// wins, so anyone can re-check the choice. It returns nil for no bids.
func Winner(bids []*states.Bid) *states.Bid {
	var winner *states.Bid
	for _, b := range bids {
		if winner == nil || b.Price > winner.Price || (b.Price == winner.Price && b.ID < winner.ID) {
			winner = b
		}
	}
	return winner
}

// CheckBidKey returns an error unless the bid is written under its own ID and
// that ID is the ID of the transaction that writes it. Transaction IDs are
// unique, so a bidder cannot point its bid at the auction or at a rival's bid.
func CheckBidKey(key, txID string, bid *states.Bid) error {
	if bid.ID != txID {
		return errors.Errorf("bid [%s] must have the ID of its transaction [%s]", bid.ID, txID)
	}
	if key != bid.ID {
		return errors.Errorf("bid [%s] is written under key [%s]", bid.ID, key)
	}
	return nil
}

// Outcome is what a bidder learns by auditing an auction.
type Outcome string

const (
	// NotEnded means the auction has no winner yet.
	NotEnded Outcome = "not-ended"
	// Won means the bidder's own bid won.
	Won Outcome = "won"
	// Lost means the revealed bid matches its commitment and beats the bidder's.
	Lost Outcome = "lost"
	// HouseMisbehaved means the revealed bid does not match its commitment, is
	// not this auction's or not the named winner's, the bidder's own commitment
	// is gone from the ledger, or the bidder's own bid should have won.
	HouseMisbehaved Outcome = "house-misbehaved"
)

// Audit checks auction from the point of view of the bidder that placed own.
// committed is the value the ledger holds under the winning bid's key,
// ownCommitted the value it holds under own's key.
func Audit(auction *states.Auction, committed, ownCommitted []byte, own *states.Bid) (Outcome, error) {
	if own.AuctionID != auction.ID {
		return "", errors.Errorf("bid [%s] is not for auction [%s]", own.ID, auction.ID)
	}
	if auction.Status != states.Ended {
		return NotEnded, nil
	}
	ownExpected, err := Commitment(own)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(ownCommitted, ownExpected) {
		return HouseMisbehaved, nil
	}
	digest := sha256.Sum256(auction.WinningBid)
	if !bytes.Equal(digest[:], committed) {
		return HouseMisbehaved, nil
	}
	revealed := &states.Bid{}
	if err := json.Unmarshal(auction.WinningBid, revealed); err != nil {
		return "", errors.Wrap(err, "failed decoding the winning bid")
	}
	if revealed.AuctionID != auction.ID || !auction.Winner.Equal(revealed.Bidder) {
		return HouseMisbehaved, nil
	}
	switch {
	case revealed.ID == own.ID:
		return Won, nil
	case Winner([]*states.Bid{revealed, own}) == own:
		return HouseMisbehaved, nil
	default:
		return Lost, nil
	}
}
