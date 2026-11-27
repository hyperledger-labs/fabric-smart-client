/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// The views the promoter runs.

// newTransaction starts a transaction in the demo's namespace with one command.
func newTransaction(viewCtx view.Context, command string, party view.Identity) (*state.Transaction, *fabric.RWSet, error) {
	tx, err := state.NewTransaction(viewCtx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed creating a new transaction")
	}
	tx.SetNamespace(Namespace)
	if err := tx.AddCommand(command, party); err != nil {
		return nil, nil, errors.Wrapf(err, "failed adding command [%s]", command)
	}
	rws, err := tx.Namespace.RWSet()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed getting the read-write set")
	}
	return tx, rws, nil
}

// submitOwn endorses a transaction with this node's own identity, orders it
// and waits until it is committed: the promoter is the namespace's only
// endorser, so its own transactions need nobody else.
func submitOwn(viewCtx view.Context, tx *state.Transaction, me view.Identity) error {
	if _, err := viewCtx.RunView(state.NewCollectEndorsementsView(tx, me)); err != nil {
		return errors.WithMessage(err, "failed endorsing")
	}
	if _, err := viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, time.Minute)); err != nil {
		return errors.WithMessage(err, "failed ordering")
	}
	return nil
}

func myIdentity(viewCtx view.Context) (view.Identity, error) {
	fns, _, err := fabric.GetDefaultChannel(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the default network")
	}
	return fns.IdentityProvider().DefaultIdentity(), nil
}

func putJSON(rws *fabric.RWSet, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return errors.Wrapf(err, "failed encoding [%s]", key)
	}
	return rws.SetState(Namespace, key, raw)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // crypto/rand.Read never fails
	return hex.EncodeToString(b)
}

// ponytail: one create at a time; fine for a promoter creating events by hand.
// Two creates in parallel would both read the same event list, and the second
// would fail at commit.
var createMu sync.Mutex

// CreateEventView creates an event and returns it with its new ID. It reads
// the event list, appends the new ID and writes both keys in one transaction.
type CreateEventView struct {
	states.Event
}

func (c *CreateEventView) Call(viewCtx view.Context) (any, error) {
	e := c.Event
	e.ID = randomHex(8)
	if err := ValidateEvent(&e); err != nil {
		return nil, err
	}
	me, err := myIdentity(viewCtx)
	if err != nil {
		return nil, err
	}

	createMu.Lock()
	defer createMu.Unlock()

	tx, rws, err := newTransaction(viewCtx, "create", me)
	if err != nil {
		return nil, err
	}
	var ids []string
	raw, err := rws.GetState(Namespace, states.EventsKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed reading the event list")
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &ids); err != nil {
			return nil, errors.Wrap(err, "failed decoding the event list")
		}
	}
	if err := putJSON(rws, states.EventsKey, append(ids, e.ID)); err != nil {
		return nil, err
	}
	if err := putJSON(rws, states.EventKey(e.ID), &e); err != nil {
		return nil, err
	}
	if err := submitOwn(viewCtx, tx, me); err != nil {
		return nil, errors.WithMessage(err, "failed creating the event")
	}
	return &e, nil
}

// UpdateEventView replaces an event's name, artist, venue, date and price. The
// number of seats never changes. A sale waiting to commit read the old version
// of the event, so it fails at commit.
type UpdateEventView struct {
	states.Event
}

func (u *UpdateEventView) Call(viewCtx view.Context) (any, error) {
	me, err := myIdentity(viewCtx)
	if err != nil {
		return nil, err
	}
	tx, rws, err := newTransaction(viewCtx, "update", me)
	if err != nil {
		return nil, err
	}
	raw, err := rws.GetState(Namespace, states.EventKey(u.ID))
	if err != nil {
		return nil, errors.Wrapf(err, "failed reading event [%s]", u.ID)
	}
	if len(raw) == 0 {
		return nil, errors.Wrapf(ErrUnknownEvent, "event [%s]", u.ID)
	}
	e := &states.Event{}
	if err := json.Unmarshal(raw, e); err != nil {
		return nil, errors.Wrapf(err, "failed decoding event [%s]", u.ID)
	}
	e.Name, e.Artist, e.Venue, e.Date, e.Price = u.Name, u.Artist, u.Venue, u.Date, u.Price
	if err := ValidateEvent(e); err != nil {
		return nil, err
	}
	if err := putJSON(rws, states.EventKey(e.ID), e); err != nil {
		return nil, err
	}
	if err := submitOwn(viewCtx, tx, me); err != nil {
		return nil, errors.WithMessagef(err, "failed updating event [%s]", e.ID)
	}
	return e, nil
}

// SaleResponderView is the promoter's side of BuyView. It checks the sale and
// endorses it; an error it returns reaches the reseller instead. It keeps no
// seat locks: if two resellers ask for the same seat at the same time, it may
// endorse both, and the ledger commits only the first.
type SaleResponderView struct{}

func (*SaleResponderView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.ReceiveTransaction(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed receiving the sale")
	}
	// The communication layer authenticated the caller; its resolver names the
	// FSC node.
	resolver, err := endpoint.GetService(viewCtx).GetResolver(viewCtx.Context(), viewCtx.Session().Info().Caller)
	if err != nil {
		return nil, errors.Wrap(err, "failed resolving the caller")
	}

	sale, err := saleTxOf(tx)
	if err != nil {
		return nil, err
	}
	seat, err := checkAgainstLedger(viewCtx, sale, resolver.Name)
	if err != nil {
		return nil, errors.WithMessagef(err, "refused sale [%s]", tx.ID())
	}
	if _, err := viewCtx.RunView(state.NewEndorseView(tx)); err != nil {
		return nil, errors.WithMessagef(err, "failed endorsing the sale of seat %d of event [%s]", seat.N, seat.Event)
	}
	return nil, nil
}

// saleTxOf extracts what CheckSale looks at from a buy transaction.
func saleTxOf(tx *state.Transaction) (SaleTx, error) {
	namespaces, err := tx.Namespaces()
	if err != nil {
		return SaleTx{}, errors.Wrap(err, "failed reading the namespaces")
	}
	if !slices.Equal(namespaces, []string{Namespace}) {
		return SaleTx{}, errors.Wrapf(ErrRefused, "a sale touches only namespace [%s], got %v", Namespace, namespaces)
	}
	rws, err := tx.Namespace.RWSet()
	if err != nil {
		return SaleTx{}, errors.Wrap(err, "failed reading the read-write set")
	}
	sale := SaleTx{Reads: map[string][]byte{}, Writes: map[string][]byte{}}
	for i := range rws.NumReads(Namespace) {
		key, err := rws.GetReadKeyAt(Namespace, i)
		if err != nil {
			return SaleTx{}, errors.Wrapf(err, "failed reading read %d", i)
		}
		version, err := rws.GetReadVersion(Namespace, key)
		if err != nil {
			return SaleTx{}, errors.Wrapf(err, "failed reading the version of [%s]", key)
		}
		if len(version) == 0 {
			version = nil
		}
		sale.Reads[key] = version
	}
	for i := range rws.NumWrites(Namespace) {
		key, value, err := rws.GetWriteAt(Namespace, i)
		if err != nil {
			return SaleTx{}, errors.Wrapf(err, "failed reading write %d", i)
		}
		sale.Writes[key] = value
	}
	return sale, nil
}

// checkAgainstLedger reads the event and the seat from the ledger, never from
// the transaction, and runs CheckSale.
func checkAgainstLedger(viewCtx view.Context, sale SaleTx, caller string) (*states.Seat, error) {
	var seatKey string
	for k := range sale.Writes {
		seatKey = k
	}
	eventID, _, ok := states.ParseSeatKey(seatKey)
	if !ok || len(sale.Writes) != 1 {
		return CheckSale(sale, &states.Event{}, false, caller)
	}
	res, err := readStates(viewCtx, states.EventKey(eventID), seatKey)
	if err != nil {
		return nil, err
	}
	var event *states.Event
	if raw, found := res[states.EventKey(eventID)]; found {
		event = &states.Event{}
		if err := json.Unmarshal(raw, event); err != nil {
			return nil, errors.Wrapf(err, "failed decoding event [%s]", eventID)
		}
	}
	_, sold := res[seatKey]
	return CheckSale(sale, event, sold, caller)
}
