# Trading on Chaguar

This example is a small economy of four producers who trade goods through a
market. A mine produces gold and a forest produces wood. A refinery turns gold
into bars and a mill turns wood into planks. Each producer is an FSC node, and
so is the market. The goods and coins are recorded on a Chaguar ledger.

The example is about how nodes work with each other. A buyer finds a listing on
the market's notice board, contacts the seller's node directly, and the two
haggle over the price. Nobody else sees the haggling. When they agree, they
sign one transaction that moves the goods one way and the coins the other.
Either all of it commits or none of it does.

For the programming model itself, read the
[Programming model guide][programming-model] first.

[programming-model]: ../../docs/platform/view/programming-model.md

## The parties

| FSC node   | Organization | Role                                                              |
| ---------- | ------------ | ----------------------------------------------------------------- |
| `market`   | `Org1`       | runs the notice board; validates and endorses every transaction   |
| `mine`     | `Org2`       | produces gold                                                     |
| `forest`   | `Org2`       | produces wood                                                     |
| `refinery` | `Org2`       | refines gold into bars, at 2 units of gold per bar                |
| `mill`     | `Org2`       | mills wood into planks, at 2 units of wood per plank              |

The network has one namespace, `trading`. Its endorsement policy requires
`Org1`, so the market's FSC node is the only endorser. It does the job that
chaincode does in Fabric, and its checks are plain Go functions in
[`views/rules.go`](views/rules.go). [`topology.go`](topology.go) defines the
network and registers each node's views. Every producer runs the same views.
What each one may produce or refine is decided by the market's registry.

## What is on the ledger

[`states/states.go`](states/states.go) defines four states, all stored in
clear:

- a lot is a quantity of one kind of goods with one owner, such as
  10 gold owned by the mine. A lot is always sold whole;
- a balance holds one producer's coins;
- a listing offers a lot for sale at an asking price. It is `open` until
  the lot is sold, and then records the agreed price;
- the registry gives each producer its role. The market writes it once, at
  setup.

## Setting up

The market's `SetupView` writes the registry and a balance of 1000 coins for
each producer in one transaction. The market is the only party, so it builds
the transaction, endorses it itself, and submits it. Setup runs once. A second
call fails.

## Producing, refining, listing

These three steps involve one producer and the market. The producer builds the
transaction and signs it, and the market's `MarketView` checks it and endorses
it.

- `ProduceView` creates a lot of raw goods. The market checks
  that the registry lets this producer make this kind. The forest cannot mine
  gold.
- `RefineView` deletes a raw lot and creates the refined lot in
  the same transaction, so the same gold can't be refined twice. The market
  checks the rate: 10 gold make exactly 5 bars, owned by the refiner.
- `ListView` creates an `open` listing for a lot the seller owns.
  Only the asking price goes on the ledger. The seller also picks a floor (the
  lowest price it will accept) and a step (how far it comes down per round).
  Those stay in the seller node's local store and never leave it.

The market reads every state it checks from the ledger, never from the
producer's description of it.

## Trading

A trade is `BuyView` on the buyer's node, which talks to `BoardView` on the
market and to `SellerView` on the seller's node. In
[`views/trade.go`](views/trade.go) it reads top to bottom in three steps.

### 1. Browse

The buyer asks the market for open listings of a kind and quantity, say
6 gold. Chaguar's query service reads one key at a time and cannot scan the
ledger for listings. The market endorses every listing, though, so when it
endorses one it pins the listing's ID on its notice board, a list in its local
store. Before it answers, it reads every pinned listing back from the ledger
and keeps only the ones that are still `open` and whose lot the seller still
owns, so the board is only an index into the ledger. A seller can list a lot
twice, and once the lot is sold through one listing, the other one drops off
the board.

### 2. Negotiate

The buyer opens a session with the seller named on the listing and sends an
offer. From here on the two nodes talk directly, and nothing they say reaches
the ledger or the market. Chaincode has no place for this: on Fabric, every
step that touches shared state is a proposal the endorsing peers see.

Each side decides with a `Negotiator` ([`views/negotiate.go`](views/negotiate.go)),
and both follow the same rule. Each side accepts any price at least as good as
its own. Otherwise it moves one step toward its limit and counters. After three
counters it gives up. The buyer starts at its opening price, the seller at its
asking price. Their limits are the buyer's budget and the seller's floor, and
neither side ever learns the other's limit.

The first trade in the test goes like this. The mine asks 100 with a floor of
90 and a step of 5. The refinery opens at 80 with a budget of 100 and a step
of 10:

```text
refinery -> mine      offer   80
mine     -> refinery  counter 95
refinery -> mine      counter 90
mine     -> refinery  accept  90
```

### 3. Swap

On a deal, the buyer builds one transaction with the command `swap`. It reads
the listing, the lot and both balances, and writes all four back: the listing
`sold` at 90, the lot owned by the refinery, 90 coins less for the refinery
and 90 more for the mine.

The buyer signs first. The transaction then goes to the seller over the same
session as the negotiation. The seller checks that it sells the listing at the
agreed price and pays the seller, then signs. Last, the market endorses it,
after checking that:

- the transaction touches exactly these four keys;
- the listing was `open` and is the seller's offer of this lot;
- the buyer can pay;
- only the listing's status and price, the lot's owner and the two balances
  change, and by exactly the price;
- buyer and seller both signed.

## Two buyers, one listing

While a seller is negotiating a listing, it holds that listing. Any other
buyer who asks about it is turned away as `busy`, and once the first deal is
final the listing is no longer `open` anyway. An honest seller never agrees to
two deals for the same lot.

A dishonest seller could still sign two swaps for one lot. The ledger stops
that. The market refuses a swap on a listing that is already `sold`. If two
swaps arrive at the same time, both read the same version of the listing, so
the one ordered second fails its version check at commit. The test shows the
hold; it does not stage a dishonest seller.

The hold lives in the seller node's memory. A restart drops it, which costs
nothing, because the ledger check still applies.

## What the market can and cannot do

The market endorses every transaction, so it has to be trusted to apply the
rules. It also sees every transaction and every browse request. It does not
see the negotiation: the offers, the counters, the buyer's budget or the
seller's floor.

The endorsement policy asks for `Org1` only, so nothing on the ledger stops the
market from writing a state on its own, for example handing itself a lot.
Producers rely on the market to apply the rules, as Fabric users rely on the
peers that run their chaincode. What the producers' signatures give them is a
record of what each of them agreed to.

## Running the example

The suite needs Docker and the Chaguar tools; see
[Integration tests](../../docs/dev/integration-tests/README.md) for the
setup. Pass an absolute `FAB_BINS`:

```bash
make integration-tests-examples-trading FAB_BINS=/absolute/path/to/fabric/bin
```

[`trading_test.go`](trading_test.go) runs the whole economy:

1. The market sets up the four producers with 1000 coins each.
2. The mine sells 10 gold to the refinery for 90.
3. The refinery makes 5 bars and sells them to the mill for 180.
4. The forest sells 20 wood to the mill for 50.
5. The mill makes 10 planks and sells them to the refinery for 130.
6. A negotiation for another 4 gold ends without a deal, twice. The listing
   stays open, and nothing moves.
7. The refinery and the mill try to buy the same 6 gold at the same time.
   Exactly one of them gets it.
8. The mill lists its bars twice and the refinery buys them. The other listing
   drops off the board.

At the end, the balances still add up to 4000 coins and every lot has the owner
the trades say it should. The rule checks and the negotiation rule have their
own unit tests in [`views/`](views/).
