/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// failureClass says what the committer does about an error raised while
// committing a block.
//
// Classification lives here rather than in whatever drives the committer because
// this is the component that can actually tell the cases apart: it owns the
// vault, so it knows which storage errors are contention that will clear; it owns
// the membership service, so it knows a configuration it could not apply will
// never apply; and it owns the channel configuration, so it knows how long to
// keep trying. A caller further up sees only an error value and would have to
// import those packages to reach the same answer.
//
// The contract that follows is the point: whatever the committer returns has
// already been retried if retrying could have helped, so an error reaching its
// caller is final and the caller can stop without inspecting it.
type failureClass int

const (
	// classNone is a nil error: the block committed.
	classNone failureClass = iota

	// classRetry is a non-deterministic failure that committing the same block
	// again can clear, typically storage contention or an I/O fault in the vault.
	// The block is re-committed with a delay between attempts; exhausting the
	// budget wraps the failure in ErrRetriesExhausted, which classifies as
	// classDegrade, because an unbounded retry on a fault that turns out to be
	// permanent is a silent stall wearing a retry's clothes.
	//
	// It follows that this class never escapes Commit: by the time an error is
	// returned, a retryable failure has either cleared or been escalated.
	classRetry

	// classDegrade is a deterministic failure on a block that is already ordered
	// and final, so committing it again produces the same error forever. The
	// error is returned, which stops the caller's block stream — a node that
	// cannot apply a block it has already accepted must not commit later ones on
	// top of it — and the failure is counted so that the stop reads as a fault
	// rather than as an absence of traffic.
	classDegrade

	// classFatal is a failure meaning an invariant this node relies on is already
	// broken, so continuing risks writing further state on top of wrong state. It
	// is counted separately from classDegrade so that an operator can tell "this
	// channel stopped" from "this node cannot be trusted", and returned like any
	// other final error.
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
// Nothing in the commit path returns it today: the failure modes reached here are
// either transient or bounded to one block or one channel, and none has been
// shown to corrupt state. It exists so that a caller which does detect such a
// condition can say so distinguishably, rather than reaching for panic from
// inside a commit.
var ErrFatalCommit = errors.New("unrecoverable commit failure")

// classify maps an error raised while committing a block to the action to take.
//
// Retry is granted only on evidence, never by default: an error must name a cause
// known to be non-deterministic — storage contention, or an expired context —
// before the same block is committed again. Everything else is a degrade, because
// retrying a deterministic failure burns the budget and arrives at the same place
// later. That is the conservative direction to be wrong in: a misclassified
// degrade stops the channel loudly, a misclassified retry spins.
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
	// escalated failure as retryable again and re-commit the block forever.
	if errors.HasCause(err, ErrRetriesExhausted) {
		return classDegrade
	}

	// Storage contention and busy-write faults are the canonical transient case.
	// Both are mapped from each backend's native codes by the SQL error wrappers,
	// so matching the sentinels covers every driver.
	if errors.HasCause(err, dbdriver.DeadlockDetected) || errors.HasCause(err, dbdriver.SqlBusy) {
		return classRetry
	}

	// An expired or cancelled context is transient with respect to the block: the
	// deadline belonged to the attempt, not to the block's contents, so a later
	// attempt under a fresh one can succeed. retryBlock re-checks the context
	// before each attempt, so a genuinely cancelled caller stops rather than
	// spending the budget.
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
	if errors.HasCause(err, driver.ErrConfigRejected) {
		return classDegrade
	}

	// Anything unrecognised degrades. Retry is granted only to causes named
	// above, so a failure this function has not been taught about stops the
	// channel loudly instead of being re-committed on the assumption it might
	// clear.
	return classDegrade
}
