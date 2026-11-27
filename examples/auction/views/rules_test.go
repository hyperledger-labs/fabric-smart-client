/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"crypto/sha256"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/examples/auction/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func bid(id string, price uint64) *states.Bid {
	return &states.Bid{
		ID:        id,
		AuctionID: "auction",
		Bidder:    view.Identity("bidder-" + id),
		Price:     price,
		Nonce:     []byte("nonce-" + id),
	}
}

func TestCheckCommitment(t *testing.T) {
	t.Parallel()

	b := bid("a", 10)
	written, err := Commitment(b)
	require.NoError(t, err)
	require.NoError(t, CheckCommitment(written, b))

	higher := *b
	higher.Price++
	require.Error(t, CheckCommitment(written, &higher))

	otherNonce := *b
	otherNonce.Nonce = []byte("another nonce")
	require.Error(t, CheckCommitment(written, &otherNonce))
}

func TestCheckBidKey(t *testing.T) {
	t.Parallel()

	b := bid("tx1", 10)
	require.NoError(t, CheckBidKey("tx1", "tx1", b))
	// A bidder may not pick the key: not the auction's, not a rival's bid.
	require.Error(t, CheckBidKey("auction", "tx1", bid("auction", 10)))
	require.Error(t, CheckBidKey("rival", "tx1", bid("rival", 10)))
	// The bid must be written under its own ID.
	require.Error(t, CheckBidKey("other", "tx1", b))
}

func TestWinner(t *testing.T) {
	t.Parallel()

	require.Nil(t, Winner(nil))
	require.Equal(t, "b", Winner([]*states.Bid{bid("a", 10), bid("b", 30), bid("c", 20)}).ID)
	require.Equal(t, "x1", Winner([]*states.Bid{bid("x2", 30), bid("x1", 30)}).ID)
}

// committed returns the value the ledger holds for b.
func committed(t *testing.T, b *states.Bid) []byte {
	t.Helper()
	c, err := Commitment(b)
	require.NoError(t, err)
	return c
}

// ended returns an ended auction revealing winner, and the hash committed for it.
func ended(t *testing.T, winner *states.Bid) (*states.Auction, []byte) {
	t.Helper()
	raw, err := json.Marshal(winner)
	require.NoError(t, err)
	committed := sha256.Sum256(raw)
	return &states.Auction{ID: "auction", Status: states.Ended, Winner: winner.Bidder, WinningBid: raw}, committed[:]
}

func TestAudit(t *testing.T) {
	t.Parallel()

	t.Run("not ended", func(t *testing.T) {
		t.Parallel()
		out, err := Audit(&states.Auction{ID: "auction", Status: states.Closed}, nil, nil, bid("a", 10))
		require.NoError(t, err)
		require.Equal(t, NotEnded, out)
	})

	t.Run("own bid won", func(t *testing.T) {
		t.Parallel()
		own := bid("a", 10)
		a, winnerHash := ended(t, own)
		out, err := Audit(a, winnerHash, committed(t, own), own)
		require.NoError(t, err)
		require.Equal(t, Won, out)
	})

	t.Run("own bid lower", func(t *testing.T) {
		t.Parallel()
		a, winnerHash := ended(t, bid("b", 30))
		out, err := Audit(a, winnerHash, committed(t, bid("a", 10)), bid("a", 10))
		require.NoError(t, err)
		require.Equal(t, Lost, out)
	})

	t.Run("revealed bid does not match its hash", func(t *testing.T) {
		t.Parallel()
		a, _ := ended(t, bid("b", 30))
		forged := sha256.Sum256([]byte("something else"))
		out, err := Audit(a, forged[:], committed(t, bid("a", 10)), bid("a", 10))
		require.NoError(t, err)
		require.Equal(t, HouseMisbehaved, out)
	})

	t.Run("own bid higher than the revealed one", func(t *testing.T) {
		t.Parallel()
		a, winnerHash := ended(t, bid("b", 30))
		out, err := Audit(a, winnerHash, committed(t, bid("a", 40)), bid("a", 40))
		require.NoError(t, err)
		require.Equal(t, HouseMisbehaved, out)
	})

	t.Run("equal price, own ID smaller", func(t *testing.T) {
		t.Parallel()
		a, winnerHash := ended(t, bid("b", 30))
		out, err := Audit(a, winnerHash, committed(t, bid("a", 30)), bid("a", 30))
		require.NoError(t, err)
		require.Equal(t, HouseMisbehaved, out)
	})

	t.Run("equal price, own ID larger", func(t *testing.T) {
		t.Parallel()
		a, winnerHash := ended(t, bid("b", 30))
		out, err := Audit(a, winnerHash, committed(t, bid("c", 30)), bid("c", 30))
		require.NoError(t, err)
		require.Equal(t, Lost, out)
	})

	t.Run("winning bid not JSON", func(t *testing.T) {
		t.Parallel()
		garbage := []byte("not json")
		digest := sha256.Sum256(garbage)
		own := bid("a", 10)
		_, err := Audit(&states.Auction{ID: "auction", Status: states.Ended, WinningBid: garbage}, digest[:], committed(t, own), own)
		require.Error(t, err)
	})

	t.Run("revealed bid from another auction", func(t *testing.T) {
		t.Parallel()
		other := bid("b", 30)
		other.AuctionID = "another auction"
		a, winnerHash := ended(t, other)
		a.ID = "auction"
		own := bid("a", 10)
		out, err := Audit(a, winnerHash, committed(t, own), own)
		require.NoError(t, err)
		require.Equal(t, HouseMisbehaved, out)
	})

	t.Run("winner named is not the revealed bid's bidder", func(t *testing.T) {
		t.Parallel()
		a, winnerHash := ended(t, bid("b", 30))
		a.Winner = view.Identity("a friend of the house")
		own := bid("a", 10)
		out, err := Audit(a, winnerHash, committed(t, own), own)
		require.NoError(t, err)
		require.Equal(t, HouseMisbehaved, out)
	})

	t.Run("own commitment missing from the ledger", func(t *testing.T) {
		t.Parallel()
		a, winnerHash := ended(t, bid("b", 30))
		out, err := Audit(a, winnerHash, nil, bid("a", 10))
		require.NoError(t, err)
		require.Equal(t, HouseMisbehaved, out)
	})

	t.Run("own bid for another auction", func(t *testing.T) {
		t.Parallel()
		a, winnerHash := ended(t, bid("b", 30))
		own := bid("a", 10)
		own.AuctionID = "another auction"
		_, err := Audit(a, winnerHash, committed(t, own), own)
		require.Error(t, err)
	})
}
