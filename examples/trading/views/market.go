/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/examples/trading/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// The views the market operator runs.

// Producer is one producer to register at setup.
type Producer struct {
	Identity view.Identity
	Role     states.Role
}

// Setup is the input of SetupView.
type Setup struct {
	Producers []Producer
	// Coins is every producer's starting balance.
	Coins uint64
}

// SetupView registers the producers and their roles, and gives each one its
// starting coins. It runs once: the market is the only party, so it builds the
// transaction, endorses it itself, and submits it.
type SetupView struct {
	Setup
}

func (s *SetupView) Call(viewCtx view.Context) (any, error) {
	if err := getState(viewCtx, states.RegistryID, &states.Registry{}); err == nil {
		return nil, errors.New("the market is already set up")
	}

	market, err := myIdentity(viewCtx)
	if err != nil {
		return nil, err
	}
	tx, err := newTransaction(viewCtx, "setup", market)
	if err != nil {
		return nil, err
	}

	registry := &states.Registry{ID: states.RegistryID, Roles: map[string]states.Role{}}
	for _, p := range s.Producers {
		registry.Roles[p.Identity.UniqueID()] = p.Role
		balance := &states.Balance{ID: states.BalanceID(p.Identity), Owner: p.Identity, Coins: s.Coins}
		if err := tx.AddOutput(balance); err != nil {
			return nil, errors.Wrap(err, "failed adding a balance")
		}
	}
	if err := tx.AddOutput(registry); err != nil {
		return nil, errors.Wrap(err, "failed adding the registry")
	}

	if err := submit(viewCtx, tx, market); err != nil {
		return nil, errors.WithMessage(err, "failed setting up the market")
	}
	return nil, nil
}

// SetupViewFactory creates a SetupView from a JSON Setup.
type SetupViewFactory struct{}

func (*SetupViewFactory) NewView(in []byte) (view.View, error) {
	f := &SetupView{}
	if err := json.Unmarshal(in, &f.Setup); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}

// MarketView is the market's side of every producer transaction. It plays the
// role a chaincode plays in Fabric: it validates the transaction and endorses
// it, but it is ordinary Go code running on the market's own node. An error
// returned here reaches the initiator, and the market does not endorse.
type MarketView struct{}

func (*MarketView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.ReceiveTransaction(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed receiving the transaction")
	}
	if err := validate(viewCtx, tx); err != nil {
		return nil, errors.WithMessagef(err, "rejected transaction [%s]", tx.ID())
	}
	if _, err := viewCtx.RunView(state.NewEndorseView(tx)); err != nil {
		return nil, errors.WithMessagef(err, "failed endorsing transaction [%s]", tx.ID())
	}
	if _, err := viewCtx.RunView(state.NewFinalityWithTimeoutView(tx, time.Minute)); err != nil {
		return nil, errors.WithMessagef(err, "transaction [%s] did not commit", tx.ID())
	}
	return nil, nil
}

// validate checks a transaction against the market's rules, by its one
// command.
func validate(viewCtx view.Context, tx *state.Transaction) error {
	namespaces, err := tx.Namespaces()
	if err != nil {
		return errors.Wrap(err, "failed reading the namespaces")
	}
	if len(namespaces) != 1 || namespaces[0] != Namespace {
		return errors.Errorf("expected only namespace [%s], got %v", Namespace, namespaces)
	}
	commands, err := tx.Commands()
	if err != nil {
		return errors.Wrap(err, "failed reading the commands")
	}
	if commands.Count() != 1 {
		return errors.Errorf("expected a single command, got %d", commands.Count())
	}
	command := commands.At(0)
	// Every party of the command must have signed.
	for _, party := range command.Ids {
		if err := tx.HasBeenEndorsedBy(party); err != nil {
			return errors.WithMessage(err, "a party has not signed")
		}
	}
	single := func() (view.Identity, error) {
		if len(command.Ids) != 1 {
			return nil, errors.Errorf("command [%s] takes one party", command.Name)
		}
		return command.Ids[0], nil
	}

	switch command.Name {
	case "produce":
		producer, err := single()
		if err != nil {
			return err
		}
		return validateProduce(viewCtx, tx, producer)
	case "refine":
		producer, err := single()
		if err != nil {
			return err
		}
		return validateRefine(viewCtx, tx, producer)
	case "list":
		seller, err := single()
		if err != nil {
			return err
		}
		return validateList(viewCtx, tx, seller)
	case "swap":
		sw, err := readSwap(tx)
		if err != nil {
			return err
		}
		return CheckSwap(sw.buyer, sw.seller, sw.before, sw.after)
	default:
		return errors.Errorf("unknown command [%s]", command.Name)
	}
}

// changes is what a transaction reads, deletes and writes, decoded.
type changes struct {
	inputs, deleted, written []string
}

func changesOf(tx *state.Transaction) (*changes, error) {
	inputs, err := tx.Inputs()
	if err != nil {
		return nil, errors.Wrap(err, "failed reading the inputs")
	}
	outputs, err := tx.Outputs()
	if err != nil {
		return nil, errors.Wrap(err, "failed reading the outputs")
	}
	c := &changes{}
	for i := range inputs.Count() {
		c.inputs = append(c.inputs, string(inputs.At(i).ID()))
	}
	for _, id := range outputs.Deleted().IDs() {
		c.deleted = append(c.deleted, string(id))
	}
	for _, id := range outputs.Written().IDs() {
		c.written = append(c.written, string(id))
	}
	return c, nil
}

// inputByKey decodes the input under key: the state as committed on the
// ledger. A read-write set keeps its keys sorted, so find inputs by key, not
// by the order they were added in.
func inputByKey(tx *state.Transaction, key string, st any) error {
	inputs, err := tx.Inputs()
	if err != nil {
		return errors.Wrap(err, "failed reading the inputs")
	}
	for i := range inputs.Count() {
		if string(inputs.At(i).ID()) == key {
			return inputs.At(i).State(st)
		}
	}
	return errors.Errorf("transaction does not read [%s]", key)
}

// outputByKey decodes the state the transaction writes under key.
func outputByKey(tx *state.Transaction, key string, st any) error {
	outputs, err := tx.Outputs()
	if err != nil {
		return errors.Wrap(err, "failed reading the outputs")
	}
	written := outputs.Written()
	for i := range written.Count() {
		if string(written.At(i).ID()) == key {
			return written.At(i).State(st)
		}
	}
	return errors.Errorf("transaction does not write [%s]", key)
}

// roleOf reads producer's role from the registry in the market's own vault,
// never from the transaction.
func roleOf(viewCtx view.Context, producer view.Identity) (states.Role, error) {
	registry := &states.Registry{}
	if err := getState(viewCtx, states.RegistryID, registry); err != nil {
		return "", err
	}
	role, ok := registry.Roles[producer.UniqueID()]
	if !ok {
		return "", errors.New("unknown producer")
	}
	return role, nil
}

func validateProduce(viewCtx view.Context, tx *state.Transaction, producer view.Identity) error {
	c, err := changesOf(tx)
	if err != nil {
		return err
	}
	lot := &states.Lot{}
	if err := tx.GetOutputAt(0, lot); err != nil {
		return errors.Wrap(err, "failed reading the lot")
	}
	if err := errors.Join(
		CheckKeys(c.inputs, nil),
		CheckKeys(c.deleted, nil),
		CheckKeys(c.written, []string{lot.ID}),
	); err != nil {
		return err
	}
	role, err := roleOf(viewCtx, producer)
	if err != nil {
		return err
	}
	return CheckProduce(role, producer, tx.ID(), lot)
}

func validateRefine(viewCtx view.Context, tx *state.Transaction, producer view.Identity) error {
	c, err := changesOf(tx)
	if err != nil {
		return err
	}
	// The raw lot as committed on the ledger, not as the producer describes it.
	raw := &states.Lot{}
	if err := tx.GetInputAt(0, raw); err != nil {
		return errors.Wrap(err, "failed reading the raw lot")
	}
	outputs, err := tx.Outputs()
	if err != nil {
		return errors.Wrap(err, "failed reading the outputs")
	}
	refined := &states.Lot{}
	if err := outputs.Written().At(0).State(refined); err != nil {
		return errors.Wrap(err, "failed reading the refined lot")
	}
	if err := errors.Join(
		CheckKeys(c.inputs, []string{raw.ID}),
		CheckKeys(c.deleted, []string{raw.ID}),
		CheckKeys(c.written, []string{refined.ID}),
	); err != nil {
		return err
	}
	role, err := roleOf(viewCtx, producer)
	if err != nil {
		return err
	}
	return CheckRefine(role, producer, tx.ID(), raw, refined)
}

// validateList checks a new listing and, if it is valid, pins it on the
// market's notice board before the market endorses it.
func validateList(viewCtx view.Context, tx *state.Transaction, seller view.Identity) error {
	c, err := changesOf(tx)
	if err != nil {
		return err
	}
	lot := &states.Lot{}
	if err := tx.GetInputAt(0, lot); err != nil {
		return errors.Wrap(err, "failed reading the lot")
	}
	listing := &states.Listing{}
	if err := tx.GetOutputAt(0, listing); err != nil {
		return errors.Wrap(err, "failed reading the listing")
	}
	if err := errors.Join(
		CheckKeys(c.inputs, []string{lot.ID}),
		CheckKeys(c.deleted, nil),
		CheckKeys(c.written, []string{listing.ID}),
	); err != nil {
		return err
	}
	if err := CheckList(seller, tx.ID(), lot, listing); err != nil {
		return err
	}
	return pin(viewCtx, listing.ID)
}

// The notice board. Chaguar's query service reads single keys only, so
// nobody can scan the ledger for listings. The market endorses every listing,
// so it remembers each one's ID in its local store and answers buyers who
// browse. The ledger stays the truth: the market reads every pinned listing
// back from it before answering, so a listing that never committed, or was
// sold, is not shown as open.

// boardPrefix groups the pinned listing IDs in the market's local store.
const boardPrefix = "board"

func pin(viewCtx view.Context, listingID string) error {
	store, err := kvsOf(viewCtx)
	if err != nil {
		return err
	}
	key, err := kvs.CreateCompositeKey(boardPrefix, []string{listingID})
	if err != nil {
		return errors.Wrap(err, "failed building the board key")
	}
	if err := store.Put(viewCtx.Context(), key, listingID); err != nil {
		return errors.Wrapf(err, "failed pinning listing [%s]", listingID)
	}
	return nil
}

// forSale says whether a committed listing can still be bought: it is open,
// and its seller still owns the lot. A seller can list a lot twice, or refine
// a listed lot; the listing then stays open on the ledger, but its lot is gone.
func forSale(viewCtx view.Context, listing *states.Listing) bool {
	if listing.Status != states.Open {
		return false
	}
	lot := &states.Lot{}
	return getState(viewCtx, listing.LotID, lot) == nil && lot.Owner.Equal(listing.Seller)
}

// Browse asks the market for open listings of a kind and quantity.
type Browse struct {
	Kind states.Kind
	Qty  uint64
	// Market is the FSC node of the market.
	Market view.Identity
}

// BrowseView asks the market for the open listings matching Browse, in
// listing ID order. Any producer can run it.
type BrowseView struct {
	Browse
}

func (b *BrowseView) Call(viewCtx view.Context) (any, error) {
	s, err := session.NewJSON(viewCtx, b, b.Market)
	if err != nil {
		return nil, errors.Wrap(err, "failed opening a session with the market")
	}
	if err := s.Send(viewCtx.Context(), &b.Browse); err != nil {
		return nil, errors.Wrap(err, "failed asking the market")
	}
	var listings []*states.Listing
	if err := s.ReceiveWithTimeout(&listings, time.Minute); err != nil {
		return nil, errors.Wrap(err, "failed receiving the listings")
	}
	return listings, nil
}

// BrowseViewFactory creates a BrowseView from a JSON Browse.
type BrowseViewFactory struct{}

func (*BrowseViewFactory) NewView(in []byte) (view.View, error) {
	f := &BrowseView{}
	if err := json.Unmarshal(in, &f.Browse); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}

// BoardView is the market's side of BrowseView.
type BoardView struct{}

func (*BoardView) Call(viewCtx view.Context) (any, error) {
	s := session.JSON(viewCtx)
	browse := &Browse{}
	if err := s.ReceiveWithTimeout(browse, time.Minute); err != nil {
		return nil, errors.Wrap(err, "failed receiving the request")
	}

	store, err := kvsOf(viewCtx)
	if err != nil {
		return nil, err
	}
	it, err := store.GetByPartialCompositeID(viewCtx.Context(), boardPrefix, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed reading the board")
	}
	defer func() { _ = it.Close() }()

	listings := []*states.Listing{}
	for it.HasNext() {
		var listingID string
		if _, err := it.Next(&listingID); err != nil {
			return nil, errors.Wrap(err, "failed reading the board")
		}
		listing := &states.Listing{}
		if err := getState(viewCtx, listingID, listing); err != nil {
			continue // pinned, but never committed
		}
		if listing.Kind == browse.Kind && listing.Qty == browse.Qty && forSale(viewCtx, listing) {
			listings = append(listings, listing)
		}
	}
	if err := s.Send(viewCtx.Context(), listings); err != nil {
		return nil, errors.Wrap(err, "failed answering")
	}
	return nil, nil
}
