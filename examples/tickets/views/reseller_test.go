/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
)

func TestClassifyRefusal(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		err  error
		want error
	}{
		{"promoter busy", errors.Wrap(session.ErrBusy, "opening session"), ErrUnreachable},
		{"promoter draining", errors.Wrap(session.ErrDraining, "opening session"), ErrUnreachable},
		{"session closed", errors.Wrap(comm.ErrSessionClosed, "receiving"), ErrUnreachable},
		{"timeout", errors.Wrap(context.DeadlineExceeded, "waiting"), ErrUnreachable},
		{"cancelled", errors.Wrap(context.Canceled, "waiting"), ErrUnreachable},
		{"unknown event", errors.New("refused sale [tx1]: event [e1]: unknown event or seat"), ErrUnknownEvent},
		{"promoter down", errors.Wrap(errors.New("dial tcp: connection refused"), "failed sending transaction content"), ErrUnreachable},
		{"seat sold", errors.New("refused sale [tx1]: seat 3 of event [e1]: seat already sold"), ErrSeatSold},
		{"other rule", errors.New("refused sale [tx1]: seat price 40 is not the event's price 50"), ErrRefused},
	} {
		require.ErrorIs(t, classifyRefusal(c.err), c.want, c.name)
	}
}

func TestCommitOutcome(t *testing.T) {
	t.Parallel()
	seat := func(ticket string) []byte {
		raw, err := json.Marshal(states.Seat{Event: "e1", N: 3, Reseller: "reseller-us", Ticket: ticket})
		require.NoError(t, err)
		return raw
	}
	commitErr := errors.New("waiting for finality failed")
	committed, invalid := int32(committerpb.Status_COMMITTED), int32(committerpb.Status_ABORTED_MVCC_CONFLICT)

	// Waiting failed, but the ledger committed the sale.
	require.NoError(t, commitOutcome(committed, true, nil, "ours", commitErr))
	// The ledger has not decided yet: the outcome is unknown, not a failure.
	require.ErrorIs(t, commitOutcome(0, false, nil, "ours", commitErr), ErrPending)
	// The ledger rejected the sale: who holds the seat now tells why.
	require.ErrorIs(t, commitOutcome(invalid, true, seat("theirs"), "ours", commitErr), ErrLostRace)
	require.ErrorIs(t, commitOutcome(invalid, true, nil, "ours", commitErr), ErrConflict)
}
