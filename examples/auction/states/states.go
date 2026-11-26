/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package states holds the ledger states of the sealed-bid auction demo.
package states

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

// Status is the phase an auction is in.
type Status string

const (
	// Open auctions accept bids.
	Open Status = "open"
	// Closed auctions accept no more bids and wait for the reveal.
	Closed Status = "closed"
	// Ended auctions name their winner.
	Ended Status = "ended"
)

// Auction is stored on the ledger in clear.
type Auction struct {
	ID     string
	Item   string
	Status Status
	// Winner and WinningBid are set when the auction ends. WinningBid is the
	// JSON encoding of the winning Bid: exactly the bytes whose SHA-256 digest
	// the ledger holds under the bid's key.
	Winner     view.Identity
	WinningBid []byte
}

// SetLinearID sets the auction's ID unless it already has one.
func (a *Auction) SetLinearID(id string) string {
	if len(a.ID) == 0 {
		a.ID = id
	}
	return a.ID
}

// Bid is stored on the ledger as the SHA-256 digest of its JSON encoding.
// Nonce is random, so the digest reveals nothing about the price.
type Bid struct {
	ID        string
	AuctionID string
	Bidder    view.Identity
	Price     uint64
	Nonce     []byte
}

// SetLinearID sets the bid's ID unless it already has one.
func (b *Bid) SetLinearID(id string) string {
	if len(b.ID) == 0 {
		b.ID = id
	}
	return b.ID
}
