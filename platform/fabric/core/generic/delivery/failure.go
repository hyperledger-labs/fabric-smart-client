/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// failureClass says what a Delivery does about an error returned by its block
// callback.
//
// The block stream is the only way a node learns that its transactions were
// committed, so how a commit failure is handled decides whether the node
// recovers, stalls visibly, or stalls silently. A single response for every
// error cannot be right: a storage deadlock clears on its own, a config block
// this node cannot parse never will, and a vault whose state cannot be
// reconciled must not have more blocks written on top of it. Classifying keeps
// those apart, and keeps the decision in one place rather than spread across the
// committer's call sites.
type failureClass int

const (
	// classNone is a nil error: the block committed.
	classNone failureClass = iota

	// classRetry is a non-deterministic failure that replaying the same block
	// can clear, typically storage contention or an I/O fault in the vault. The
	// block is re-submitted with backoff; exhausting the budget wraps the failure
	// in ErrRetriesExhausted, which classifies as classDegrade, because an
	// unbounded retry on a fault that turns out to be permanent is a silent stall
	// wearing a retry's clothes.
	//
	// It follows that this class is never recorded against a stopped channel: by
	// the time delivery stops, a retryable failure has been escalated.
	classRetry

	// classDegrade is a deterministic failure on a block that is already
	// ordered and final, so replaying it produces the same error forever.
	// Delivery stops, because a node that cannot apply a block it has already
	// accepted must not commit later ones on top of it — but the stop is
	// recorded and counted, so that it reads as a fault rather than as an
	// absence of traffic.
	classDegrade

	// classFatal is a failure meaning an invariant this node relies on is
	// already broken, so continuing risks writing further state on top of wrong
	// state. Delivery stops and reports the condition to whatever the embedding
	// process installed to handle it.
	classFatal
)

func (c failureClass) String() string {
	switch c {
	case classNone:
		return "none"
	case classRetry:
		return "retry"
	case classDegrade:
		return "degrade"
	case classFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

// FatalHandler is called when a block commit fails in a way that leaves the node
// unable to trust its own committed state. It is given the network and channel
// the failure happened on and the error that caused it.
//
// The handler decides what to do about the process: a typical one logs, flushes,
// and exits non-zero so that a supervisor restarts the node. It is called from
// the delivery goroutine after that channel's delivery has already been stopped,
// and must not block indefinitely.
type FatalHandler func(network, channel string, err error)

// ErrRetriesExhausted marks a transient commit failure that did not clear within
// its retry budget.
//
// It changes the failure's class rather than its cause. The error carrying it
// classifies as classDegrade, because a fault that survived every retry is no
// longer usefully described as transient — without it, an exhausted budget would
// keep reporting the class it started in, and an operator alerting on the degrade
// class would miss the very case the retry exists to handle.
//
// The original cause remains matchable with errors.Is alongside it, so a caller
// can still tell what actually failed. classify relies on that: it tests for this
// sentinel before the retryable ones precisely because both are reachable.
var ErrRetriesExhausted = errors.New("commit retries exhausted")

// ErrFatalCommit marks an error whose cause leaves this node unable to trust its
// own committed state, so that block processing must not continue over it.
//
// Nothing in the commit path returns it today: the committer's failure modes are
// either transient or bounded to one block or one channel, and none has been
// shown to corrupt state. It exists so that a caller which does detect such a
// condition can say so distinguishably, rather than reaching for panic and
// taking the process down from inside a block callback.
var ErrFatalCommit = errors.New("unrecoverable commit failure")

// classify maps an error returned by a block callback to the action to take.
//
// Retry is granted only on evidence, never by default: an error must name a
// cause known to be non-deterministic — storage contention, or an expired
// context — before the same block is replayed. Everything else is a degrade,
// because retrying a deterministic failure burns the budget and arrives at the
// same place later. That is the conservative direction to be wrong in: a
// misclassified degrade stalls the channel loudly, a misclassified retry spins.
func classify(err error) failureClass {
	if err == nil {
		return classNone
	}

	// Fatal is checked first: a wrapped fatal cause outranks anything else the
	// error might also match.
	if errors.HasCause(err, ErrFatalCommit) {
		return classFatal
	}

	// Checked before the retryable causes, and load-bearing rather than
	// defensive: an exhausted budget deliberately keeps the transient error that
	// caused it matchable, so testing for contention first would classify the
	// escalated failure as retryable again and replay the block forever.
	if errors.HasCause(err, ErrRetriesExhausted) {
		return classDegrade
	}

	// Storage contention and busy-write faults are the canonical transient
	// case. Both are mapped from each backend's native codes by the SQL error
	// wrappers, so matching the sentinels covers every driver.
	if errors.HasCause(err, dbdriver.DeadlockDetected) || errors.HasCause(err, dbdriver.SqlBusy) {
		return classRetry
	}

	// An expired or cancelled context is transient with respect to the block: the
	// deadline belonged to the attempt, not to the block's contents, so a later
	// attempt under a fresh one can succeed.
	//
	// A shutdown can still reach here, rather than only through d.stop: when the
	// context Run was given is cancelled, runReceiver calls Stop while a callback
	// may already be in flight on readBlocks, so the cancellation can surface as
	// a callback error too. That is why invokeCallback re-checks d.stop before
	// every attempt — the retry is abandoned on the next pass rather than being
	// prevented from starting.
	if errors.HasCause(err, context.DeadlineExceeded) || errors.HasCause(err, context.Canceled) {
		return classRetry
	}

	// A configuration this node refused is deterministic by construction: the
	// membership service parses the envelope with no I/O, so the same bytes fail
	// the same way however often they are replayed.
	//
	// This branch reaches the same answer as the fallthrough below and so changes
	// no behaviour. It is kept because it is the case both #1624 and #1731 are
	// about, and a reader deciding how a rejected configuration is treated should
	// find it stated rather than inferred from a default.
	if errors.HasCause(err, fdriver.ErrConfigRejected) {
		return classDegrade
	}

	// Anything unrecognised degrades. Retry is granted only to causes named
	// above, so a failure this function has not been taught about stops the
	// channel loudly instead of being replayed on the assumption it might clear.
	return classDegrade
}
