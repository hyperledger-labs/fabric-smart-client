/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

// Negotiation. Buyer and seller exchange Messages in a session between their
// two nodes; nothing here reaches the ledger. Each side decides with a
// Negotiator, a deterministic rule: accept any price at least as good as your
// own, otherwise move one step toward your limit and counter, and give up
// after MaxCounters counters.

// Decision is what a Message says.
type Decision string

const (
	// Offer opens a negotiation: the buyer's first price for a listing.
	Offer Decision = "offer"
	// Counter proposes another price.
	Counter Decision = "counter"
	// Accept agrees to the other side's last price.
	Accept Decision = "accept"
	// Reject ends the negotiation without a deal.
	Reject Decision = "reject"
)

// Message is one step of a negotiation. ListingID is set only on the Offer;
// Reason only on a Reject.
type Message struct {
	Decision  Decision
	ListingID string
	Price     uint64
	Reason    string
}

// MaxCounters is how many times each side may counter.
const MaxCounters = 3

// Negotiator is one side's position. Price is its current price: the ask for a
// seller, the opening offer for a buyer. Limit is the seller's floor or the
// buyer's budget, and Step how far it concedes per counter.
type Negotiator struct {
	Selling  bool
	Price    uint64
	Limit    uint64
	Step     uint64
	Counters int
}

// Respond answers the other side's price.
func (n *Negotiator) Respond(theirs uint64) Message {
	if n.good(theirs) {
		return Message{Decision: Accept, Price: theirs}
	}
	if n.Counters == MaxCounters {
		return Message{Decision: Reject, Reason: "no deal"}
	}
	if n.Selling {
		n.Price = max(n.Limit, n.Price-min(n.Step, n.Price))
	} else {
		n.Price = min(n.Limit, n.Price+n.Step)
	}
	if n.good(theirs) {
		return Message{Decision: Accept, Price: theirs}
	}
	n.Counters++
	return Message{Decision: Counter, Price: n.Price}
}

// good says whether price is at least as good as this side's own. Nothing
// is sold for free, so 0 is never good.
func (n *Negotiator) good(price uint64) bool {
	if price == 0 {
		return false
	}
	if n.Selling {
		return price >= n.Price
	}
	return price <= n.Price
}
