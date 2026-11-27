/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// step is one answer in a negotiation: who gave it and what it was.
type step struct {
	seller   bool
	decision Decision
	price    uint64
}

// negotiate plays a session: the buyer's opening price goes to the seller,
// then the two answer each other until one accepts or rejects.
func negotiate(seller, buyer Negotiator) []step {
	var steps []step
	price, sellerTurn := buyer.Price, true
	for {
		n := &buyer
		if sellerTurn {
			n = &seller
		}
		m := n.Respond(price)
		steps = append(steps, step{sellerTurn, m.Decision, m.Price})
		if m.Decision != Counter {
			return steps
		}
		price, sellerTurn = m.Price, !sellerTurn
	}
}

func sell(ask, floor, step uint64) Negotiator {
	return Negotiator{Selling: true, Price: ask, Limit: floor, Step: step}
}

func buy(opening, budget, step uint64) Negotiator {
	return Negotiator{Price: opening, Limit: budget, Step: step}
}

func TestNegotiate(t *testing.T) {
	t.Parallel()

	const s, b = true, false
	for name, tc := range map[string]struct {
		seller, buyer Negotiator
		want          []step
	}{
		"gold": {sell(100, 90, 5), buy(80, 100, 10), []step{
			{s, Counter, 95}, {b, Counter, 90}, {s, Accept, 90},
		}},
		"bars": {sell(200, 180, 10), buy(150, 200, 20), []step{
			{s, Counter, 190}, {b, Counter, 170}, {s, Counter, 180}, {b, Accept, 180},
		}},
		"wood": {sell(60, 50, 5), buy(40, 60, 5), []step{
			{s, Counter, 55}, {b, Counter, 45}, {s, Counter, 50}, {b, Accept, 50},
		}},
		"planks": {sell(150, 130, 10), buy(120, 150, 5), []step{
			{s, Counter, 140}, {b, Counter, 125}, {s, Counter, 130}, {b, Accept, 130},
		}},
		"no deal": {sell(100, 90, 5), buy(60, 70, 5), []step{
			{s, Counter, 95},
			{b, Counter, 65},
			{s, Counter, 90},
			{b, Counter, 70},
			{s, Counter, 90},
			{b, Counter, 70},
			{s, Reject, 0},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, negotiate(tc.seller, tc.buyer))
		})
	}
}

func TestNegotiatorRejectReason(t *testing.T) {
	t.Parallel()

	n := Negotiator{Selling: true, Price: 100, Limit: 100, Step: 5, Counters: MaxCounters}
	require.Equal(t, Message{Decision: Reject, Reason: "no deal"}, n.Respond(10))
}

func TestNegotiatorUnderflow(t *testing.T) {
	t.Parallel()

	n := sell(3, 0, 5)
	require.Equal(t, Message{Decision: Accept, Price: 1}, n.Respond(1))
	require.Zero(t, n.Price)
}

func TestNegotiatorNeverAcceptsZero(t *testing.T) {
	t.Parallel()

	seller := sell(10, 0, 10)
	require.NotEqual(t, Accept, seller.Respond(0).Decision)
	buyer := buy(0, 10, 5)
	require.NotEqual(t, Accept, buyer.Respond(0).Decision)
}
