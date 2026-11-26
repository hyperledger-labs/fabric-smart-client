# Sealed-bid auction on Chaguar

This example runs a sealed-bid auction between an auction house and three
bidders. Each party is an FSC node, and each step of the auction is a pair of
views: one party starts it, the other answers. The auction is recorded on a
Chaguar ledger. While the auction is open, the ledger holds only a hash of each
bid. When it ends, the house publishes the winning bid, and every bidder can
check it against the hash.

The scenario follows the Fabric sample [`auction-simple`][auction-simple], which
builds the same auction as chaincode. The
[comparison](#compared-with-the-chaincode-sample) at the end shows what changes
when the business process is written as views. For the programming model
itself, read the [Programming model guide][programming-model] first.

[auction-simple]:
  https://github.com/hyperledger/fabric-samples/tree/main/auction-simple
[programming-model]: ../../docs/platform/view/programming-model.md

## The parties

| FSC node                     | Organization | Role                                                       |
| ---------------------------- | ------------ | ---------------------------------------------------------- |
| `house`                      | `Org1`       | runs the auction; validates and endorses every transaction |
| `alice`, `bob` and `charlie` | `Org2`       | bid                                                        |

The network has one namespace, `auction`. Its endorsement policy requires
`Org1`, so the house's FSC node is the only endorser. It plays the part of the
chaincode, and its checks are Go code in a view. There is no separate
approver node. [`topology.go`](topology.go) defines the network and registers
each node's views.

## The protocol

| Step   | Started by | Answered by | Ledger effect                                       |
| ------ | ---------- | ----------- | --------------------------------------------------- |
| Create | house      | nobody      | writes the auction, `open`                          |
| Bid    | bidder     | house       | reads the auction, writes the hash of the bid       |
| Close  | house      | nobody      | rewrites the auction, `closed`                      |
| Reveal | house      | nobody      | rewrites the auction, `ended`, with the winning bid |
| Audit  | bidder     | nobody      | none: the bidder only reads                         |

The two states are in [`states/states.go`](states/states.go). The auction
rules that every party applies (the hash check, the choice of the winner and
the audit) are plain functions in [`views/rules.go`](views/rules.go), unit
tested without a network.

### Create an auction

`CreateAuctionView` in [`views/house.go`](views/house.go) builds a transaction
that writes an `Auction` with status `open`. The house is the only party, so
it endorses its own transaction and submits it:

```go
auction := &states.Auction{Item: c.Item, Status: states.Open}
if err := tx.AddOutput(auction); err != nil {
	return nil, errors.Wrap(err, "failed adding the auction")
}
if err := submit(viewCtx, tx, house); err != nil {
	return nil, errors.WithMessage(err, "failed creating the auction")
}
```

### Place a sealed bid

A bid is one transaction that the bidder builds and both parties sign.
`PlaceBidView` in [`views/bidder.go`](views/bidder.go):

1. Reads the auction as an input. The bid then depends on the auction's
   current version, so a bid that races the close fails at commit.
1. Adds the bid with `state.WithHashHiding()`, under the transaction's ID.
   The ledger gets the SHA-256 hash of the bid's JSON encoding. The bid itself
   travels to the house as transient data, which is never ordered or stored on
   the ledger.
1. Collects the signatures of the bidder and the house, then submits.
1. Keeps its own bid in its node's local store, to audit the reveal later.

```go
bid := &states.Bid{ID: tx.ID(), AuctionID: auction.ID, Bidder: me, Price: p.Price, Nonce: nonce}
if err := tx.AddOutput(bid, state.WithHashHiding()); err != nil {
	return nil, errors.Wrap(err, "failed adding the bid")
}
if err := submit(viewCtx, tx, me, p.House); err != nil {
	return nil, errors.WithMessage(err, "failed placing the bid")
}
```

The house answers with `BidResponderView`. It receives the transaction and runs
`validateBid`, which does what the chaincode does in Fabric. It checks that the
auction is open, that the bid is the only output, that the bidder signed, and
that the price is positive. It also checks that the bid is written under its
transaction's ID, and that the bid it sees hashes to the value written on the
ledger. If any check fails, the responder returns the error to the bidder and
does not endorse:

```go
key, written, err := rws.GetWriteAt(Namespace, 0)
if err != nil {
	return nil, errors.Wrap(err, "failed reading the bid's write")
}
if err := CheckBidKey(key, tx.ID(), bid); err != nil {
	return nil, err
}
if err := CheckCommitment(written, bid); err != nil {
	return nil, err
}
```

Transaction IDs are unique, so the key check stops a bidder from writing its
bid over the auction or over a rival's bid. The state service does not make
the hash check by itself. Without it, a bidder could commit the hash of one
price and show the house another. The house then records the bid in its local
store, endorses the transaction, and waits for it to commit. The ledger now
holds the bid's hash in a transaction that the bidder signed and the house
endorsed.

### Close the auction

`CloseAuctionView` rewrites the auction with status `closed`. From then on the
house refuses new bids. A bid that read the open auction before the close fails
at commit, because the auction's version changed.

### Reveal the winner

`RevealView` reads the bids the house recorded for the auction and keeps those
whose transaction committed as valid. It picks the highest price; between equal
prices, the smallest bid ID wins, so anyone can re-check the choice. It then
rewrites the auction with status `ended`, the winner, and the winning bid's
JSON encoding: exactly the bytes whose hash the ledger holds.

### Audit the result

`AuditView` runs on a bidder's node and involves nobody else. It reads the
ended auction and the hash stored under the winning bid's key, then calls
`Audit` from [`views/rules.go`](views/rules.go), which returns one of:

- `won`: the winning bid is the bidder's own;
- `lost`: the published bid matches its hash and beats the bidder's;
- `house-misbehaved`: the published bid does not match its hash, belongs to
  another auction or to someone other than the named winner, the bidder's own
  hash is gone from the ledger, or the bidder's own bid should have won;
- `not-ended`: the auction has no winner yet.

## Why each bid has a nonce

A price is a small number. If the ledger held the hash of the price alone,
anyone could hash every plausible price and find the match. Each `Bid` carries
32 random bytes in `Nonce`, so its hash reveals nothing about the price until
the house publishes the bid.

## What the house can and cannot do

The house sees every bid in clear: bidders trust it not to leak bids while the
auction is open. In exchange, the ledger limits what it can do with them:

- It cannot change a bid after the fact: the bid's hash is on the ledger, and
  the audit checks the published bid against it.
- It cannot deny or drop a bid: the ledger holds its hash in a transaction
  the bidder signed, and the audit checks that it is still there.
- It cannot quietly pick a lower bid as the winner, or name someone else as
  the winner: the audit of the bidder who placed the higher bid reports
  `house-misbehaved`, and that bidder keeps its own bid as proof.

The audit does not catch everything. The house is the only endorser, so it can
commit bids of its own under invented bidders, even after the close, and a
fake higher bid audits as `lost` for everyone. Checking that the winning bid's
transaction carries the named bidder's signature would close this gap; the
example leaves it out.

Acting on a `house-misbehaved` result, for example with a dispute transaction,
is outside this example.

## Compared with the chaincode sample

| Concern              | `auction-simple` (chaincode)                                                                                     | This example (views)                                                                                       |
| -------------------- | ---------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| Keeping bids private | The bid goes into the implicit private data collection of the bidder's organization, passed in as transient data | The bid goes from the bidder's view to the house's view in transient data, with no collection to configure |
| Placing a bid        | Two transactions: `bid` stores the bid, `submitBid` adds its hash to the auction                                 | One transaction, signed by the bidder and endorsed by the house                                            |
| Validation           | Chaincode, packaged and deployed to the peers                                                                    | Go code in the house's responder view                                                                      |
| Endorsement          | State-based endorsement adds each bidder's organization to the auction's policy                                  | A fixed policy: the house's organization                                                                   |
| Revealing            | Each bidder reveals its own bid; the organizations endorse the end only if no unrevealed bid would win           | The house publishes the winning bid; each bidder audits it                                                 |
| Sequencing the steps | Node.js scripts and `network.sh`, run by hand in order                                                           | Views that call each other; the test runs them with `CallView`                                             |

The two designs trust different parties. In `auction-simple`, no single party
sees every bid, and every bidding organization has to endorse the end of the
auction. Here the house sees every bid, and bidders detect cheating after the
fact instead of blocking it. The example keeps that design because it shows a
transaction that two parties build together and validation written as a view.

## Running the example

The suite needs Docker and the Chaguar tools; see
[Integration tests](../../docs/dev/integration-tests/README.md) for the
setup. Pass an absolute `FAB_BINS`:

```bash
make integration-tests-examples-auction FAB_BINS=/absolute/path/to/fabric/bin
```

[`auction_test.go`](auction_test.go) runs the whole auction: alice, bob and
charlie bid 10, 30 and 20, a bid after the close is refused, bob wins, and each
bidder audits the result.
