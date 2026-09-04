/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// txStatus is one status report for a transaction.
type txStatus struct {
	code    fdriver.ValidationCode
	message string
}

// Why this shape: a listener that reacts only to the status it hoped for turns a wrong
// answer into an unbounded wait rather than a failure. Recording whatever arrives and
// leaving the comparison to the caller makes a wrong status fail fast and name both values.

// FinalityListener records the first status reported for one transaction so that a caller
// can wait for it under its own deadline. It satisfies the finality listener contract in
// [driver.FinalityListener]: OnStatus records and returns, never blocking.
type FinalityListener struct {
	txID string
	// done is buffered with capacity one: the first status wins and later reports are
	// dropped, so a redelivery can neither block a sender nor double-signal a waiter.
	done chan txStatus
}

// NewFinalityListener returns a listener waiting for a status on txID.
func NewFinalityListener(txID string) *FinalityListener {
	return &FinalityListener{txID: txID, done: make(chan txStatus, 1)}
}

// OnStatus records the status and returns immediately. Only the first status for this
// listener's transaction is kept; later reports, and reports for any other transaction,
// are ignored.
//
// ctx is deliberately unused. [driver.FinalityListener] requires OnStatus to observe
// cancellation and return promptly, which this satisfies by construction: the send below
// cannot block, so a cancellation has nothing to abort. Acting on ctx would make it worse
// -- a done ctx means the driver's own handler deadline passed, and dropping a status the
// committer did deliver would turn a real answer into a timeout in Expect, which is bounded
// separately. Selecting on ctx.Done() alongside the default would not even be
// deterministic: both cases are always ready, so Go would pick between them at random.
func (l *FinalityListener) OnStatus(_ context.Context, txID driver.TxID, vc fdriver.ValidationCode, message string) {
	if txID != l.txID {
		return
	}
	select {
	case l.done <- txStatus{code: vc, message: message}:
	default:
	}
}

// Expect waits for a status and returns nil only if it equals want. It returns as soon as a
// status arrives, ctx is done, or timeout elapses, whichever comes first. The error names
// the transaction, and a wrong status names both codes.
//
// timeout is required and applied on top of ctx rather than left to the caller's context: a
// view's own context lives as long as the view, which is far too coarse to notice a
// transaction that never reaches finality, so every call site needs a tighter bound anyway.
// Taking it as an argument keeps that to one argument instead of a context.WithTimeout and a
// deferred cancel per call site. A non-positive timeout fails immediately.
//
// Expect consumes the recorded status, so a second call waits for a new one.
func (l *FinalityListener) Expect(ctx context.Context, want fdriver.ValidationCode, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case st := <-l.done:
		if st.code != want {
			return errors.Errorf("tx [%s]: expected status [%d], got [%d]: %s", l.txID, want, st.code, st.message)
		}
		return nil
	case <-ctx.Done():
		return errors.Wrapf(ctx.Err(), "tx [%s]: no status reported, expected [%d]", l.txID, want)
	}
}
