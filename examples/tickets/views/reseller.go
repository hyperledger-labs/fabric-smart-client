/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/hyperledger/fabric-x-common/api/committerpb"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// MaxHold bounds Buy.Hold.
const MaxHold = 10 * time.Second

// Buy is the input of BuyView.
type Buy struct {
	EventID string
	Seat    int
	// Hold is a demo-only pause between the promoter's endorsement and the
	// submission. Two resellers buying the same seat within it both get
	// endorsed, and the ledger decides which sale commits.
	Hold time.Duration
}

// BuyView sells a seat and returns it. The reseller builds the transaction,
// the promoter checks and endorses it, and the reseller submits it.
type BuyView struct {
	Buy
}

func (b *BuyView) Call(viewCtx view.Context) (any, error) {
	if b.Hold < 0 || b.Hold > MaxHold {
		return nil, errors.Wrapf(ErrInvalidInput, "hold must be 0 to %s", MaxHold)
	}
	promoter, err := endpoint.GetService(viewCtx).GetIdentity(PromoterNode, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed resolving the promoter")
	}
	me, err := myIdentity(viewCtx)
	if err != nil {
		return nil, err
	}

	tx, rws, err := newTransaction(viewCtx, "buy", me)
	if err != nil {
		return nil, err
	}
	// Both reads go into the transaction: the event's version ties the sale to
	// the price, and reading the seat while it does not exist is what makes
	// the ledger reject a second sale of it.
	raw, err := rws.GetState(Namespace, states.EventKey(b.EventID))
	if err != nil {
		return nil, errors.Wrapf(err, "failed reading event [%s]", b.EventID)
	}
	if len(raw) == 0 {
		return nil, errors.Wrapf(ErrUnknownEvent, "event [%s]", b.EventID)
	}
	event := &states.Event{}
	if err := json.Unmarshal(raw, event); err != nil {
		return nil, errors.Wrapf(err, "failed decoding event [%s]", b.EventID)
	}
	if b.Seat < 1 || b.Seat > event.Seats {
		return nil, errors.Wrapf(ErrUnknownEvent, "event [%s] has no seat %d", b.EventID, b.Seat)
	}
	seatKey := states.SeatKey(b.EventID, b.Seat)
	raw, err = rws.GetState(Namespace, seatKey)
	if err != nil {
		return nil, errors.Wrapf(err, "failed reading [%s]", seatKey)
	}
	if len(raw) > 0 {
		return nil, errors.Wrapf(ErrSeatSold, "seat %d of event [%s]", b.Seat, b.EventID)
	}

	seat := &states.Seat{
		Event:    b.EventID,
		N:        b.Seat,
		Reseller: config.GetProvider(viewCtx).ID(),
		Price:    event.Price,
		Ticket:   randomHex(16),
	}
	if err := putJSON(rws, seatKey, seat); err != nil {
		return nil, err
	}

	if _, err := viewCtx.RunView(state.NewCollectEndorsementsView(tx, promoter)); err != nil {
		return nil, classifyRefusal(err)
	}
	select {
	case <-time.After(b.Hold):
	case <-viewCtx.Context().Done():
		return nil, errors.Wrap(viewCtx.Context().Err(), "cancelled while holding the sale")
	}
	if _, err := viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, time.Minute)); err != nil {
		// Waiting can fail while the sale commits anyway: ask the ledger.
		if err := classifyCommitFailure(viewCtx, tx.ID(), seatKey, seat.Ticket, err); err != nil {
			return nil, err
		}
	}
	return seat, nil
}

// classifyRefusal tells why the sale got no endorsement. A promoter that is
// down, busy, draining or slow is unreachable. Otherwise the promoter refused,
// and its error arrives as text, so ErrSeatSold is recognised by its message.
func classifyRefusal(err error) error {
	switch {
	case errors.Is(err, session.ErrBusy), errors.Is(err, session.ErrDraining),
		errors.Is(err, comm.ErrSessionClosed), errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, context.Canceled),
		// The endorser's wrap of a local send failure: the sale never reached the promoter.
		strings.Contains(err.Error(), "failed sending transaction content"):
		return errors.Wrap(ErrUnreachable, err.Error())
	case strings.Contains(err.Error(), ErrSeatSold.Error()):
		return errors.Wrap(ErrSeatSold, err.Error())
	case strings.Contains(err.Error(), ErrUnknownEvent.Error()):
		return errors.Wrap(ErrUnknownEvent, err.Error())
	default:
		return errors.Wrap(ErrRefused, err.Error())
	}
}

// classifyCommitFailure asks the ledger what became of a sale after waiting
// for it failed: committed anyway (nil), not decided yet (ErrPending), or
// rejected, because another reseller won the race or for another reason.
func classifyCommitFailure(viewCtx view.Context, txID, seatKey, ticket string, err error) error {
	fns, ch, ferr := fabric.GetDefaultChannel(viewCtx)
	if ferr != nil {
		return errors.Wrapf(ErrPending, "ticket [%s]: %s", ticket, err)
	}
	qs, qerr := queryservice.GetQueryService(viewCtx, fns.Name(), ch.Name())
	if qerr != nil {
		return errors.Wrapf(ErrPending, "ticket [%s]: %s", ticket, err)
	}
	// A transaction the committer has not decided yet is missing from the result.
	statuses, serr := qs.GetTransactionStatuses([]string{txID})
	if serr != nil {
		return errors.Wrapf(ErrPending, "ticket [%s]: %s", ticket, err)
	}
	status, final := statuses[txID]
	var stored []byte
	if final && status != int32(committerpb.Status_COMMITTED) {
		if res, rerr := readStates(viewCtx, seatKey); rerr == nil {
			stored = res[seatKey]
		}
	}
	return commitOutcome(status, final, stored, ticket, err)
}

// commitOutcome classifies a sale whose commit wait failed, from the
// transaction's status and what the seat holds now.
func commitOutcome(status int32, final bool, stored []byte, ticket string, err error) error {
	switch {
	case !final:
		return errors.Wrapf(ErrPending, "ticket [%s]: %s", ticket, err)
	case status == int32(committerpb.Status_COMMITTED):
		return nil
	}
	if stored != nil {
		other := &states.Seat{}
		if json.Unmarshal(stored, other) == nil && other.Ticket != ticket {
			return errors.Wrapf(ErrLostRace, "seat %d of event [%s] went to reseller [%s]", other.N, other.Event, other.Reseller)
		}
	}
	return errors.Wrap(ErrConflict, err.Error())
}
