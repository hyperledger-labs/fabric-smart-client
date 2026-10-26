/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer/fake"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// testCommitRetries is the budget the retry tests pin, independent of the
// configured default, so that changing the default cannot silently change what
// these tests assert.
const testCommitRetries = 3

// newRetryCommitter builds a Committer wired only for retryBlock: a channel
// config carrying the pinned budget and a zero retry delay so the tests do not
// sleep, and real metrics over a discarding provider so the counter increments
// are exercised rather than skipped.
func newRetryCommitter(t *testing.T) *Committer {
	t.Helper()

	return &Committer{
		ChannelConfig: &fake.ChannelConfig{
			IDValue: "testChannel",
			Retries: testCommitRetries,
		},
		logger:  logger,
		metrics: NewMetrics(noop.NewTracerProvider(), &fake.MetricsProvider{}, "testNet", "testChannel"),
	}
}

// withRetrySleep sets a short, non-zero retry delay, so that a test exercises
// the waits, including the backoff on a lost connection, without slowing down.
func withRetrySleep(c *Committer) *Committer {
	c.ChannelConfig.(*fake.ChannelConfig).RetrySleep = time.Millisecond
	return c
}

func testBlock(n uint64) *common.Block {
	return &common.Block{Header: &common.BlockHeader{Number: n}}
}

// countingCounter records every increment, and the label values it was made
// under, so a test can assert how many times a counter fired rather than only
// that the call did not panic.
type countingCounter struct {
	labels []string
	onAdd  func(labels []string)
}

func (c *countingCounter) With(labelValues ...string) metrics.Counter {
	return &countingCounter{
		labels: append(append([]string{}, c.labels...), labelValues...),
		onAdd:  c.onAdd,
	}
}

func (c *countingCounter) Add(float64) {
	if c.onAdd != nil {
		c.onAdd(c.labels)
	}
}

// TestRetryBlockTransient is the regression test for issue #1731: a transient
// commit failure must not be reported to the caller, because reporting it ends
// the channel's block stream permanently.
func TestRetryBlockTransient(t *testing.T) {
	t.Parallel()

	t.Run("recovers when a transient failure clears", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		c := newRetryCommitter(t)
		c.commitBlock = func(context.Context, *common.Block) error {
			// Fail twice with a wrapped deadlock, as the vault would, then succeed.
			if calls.Add(1) <= 2 {
				return errors.Wrap(dbdriver.DeadlockDetected, "failed committing tx")
			}
			return nil
		}

		require.NoError(t, c.Commit(t.Context(), testBlock(7)))
		require.Equal(t, int32(3), calls.Load(), "the block should be committed again until it lands")
	})

	t.Run("counts exactly the retries it performs", func(t *testing.T) {
		t.Parallel()
		// With a budget of N the block is attempted N+1 times, of which N are
		// retries. Counting on failure instead would also count the final attempt,
		// which no retry follows, over-reporting by one on every exhausted block.
		var attempts, retriesCounted atomic.Int32
		c := newRetryCommitter(t)
		c.metrics.CommitRetries = &countingCounter{onAdd: func([]string) {
			retriesCounted.Add(1)
		}}
		c.commitBlock = func(context.Context, *common.Block) error {
			attempts.Add(1)
			return dbdriver.DeadlockDetected
		}

		require.Error(t, c.Commit(t.Context(), testBlock(21)))
		require.Equal(t, int32(testCommitRetries+1), attempts.Load(), "attempts is the budget plus the first try")
		require.Equal(t, int32(testCommitRetries), retriesCounted.Load(), "only the retries are counted")
	})

	t.Run("counts nothing when the first attempt succeeds", func(t *testing.T) {
		t.Parallel()
		var retriesCounted atomic.Int32
		c := newRetryCommitter(t)
		c.metrics.CommitRetries = &countingCounter{onAdd: func([]string) {
			retriesCounted.Add(1)
		}}
		c.commitBlock = func(context.Context, *common.Block) error { return nil }

		require.NoError(t, c.Commit(t.Context(), testBlock(23)))
		require.Zero(t, retriesCounted.Load())
	})

	t.Run("gives up after the budget and escalates out of the retry class", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		c := newRetryCommitter(t)
		c.commitBlock = func(context.Context, *common.Block) error {
			calls.Add(1)
			return errors.Wrap(dbdriver.SqlBusy, "still busy")
		}

		err := c.Commit(t.Context(), testBlock(9))

		require.Error(t, err)
		// One initial attempt plus the budget: bounded, so a permanent fault
		// reaches a decision instead of spinning.
		require.Equal(t, int32(testCommitRetries+1), calls.Load())
		// The class is what an operator alerts on, so assert it rather than only
		// the attempt count. A fault that survived every retry must report as a
		// degrade, or an alert on that class misses the case retrying exists for.
		require.ErrorIs(t, err, ErrRetriesExhausted)
		require.Equal(t, classDegrade, classify(err))
		// Asserted with ErrorIs, not by message: the escalation has to keep the
		// cause matchable for a caller branching on it, and a substring check
		// passes just as well when the cause has been flattened into text.
		require.ErrorIs(t, err, dbdriver.SqlBusy,
			"the original cause must stay machine-readable")
	})

	t.Run("returns a deterministic failure without retrying", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		c := newRetryCommitter(t)
		c.commitBlock = func(context.Context, *common.Block) error {
			calls.Add(1)
			return errors.Wrap(fdriver.ErrConfigRejected, "failed updating membership service")
		}

		err := c.Commit(t.Context(), testBlock(11))

		require.ErrorIs(t, err, fdriver.ErrConfigRejected)
		require.Equal(t, int32(1), calls.Load(), "a deterministic failure must not be retried")
	})

	t.Run("abandons retries when the caller's context is cancelled", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		c := newRetryCommitter(t)
		c.commitBlock = func(context.Context, *common.Block) error {
			calls.Add(1)
			return dbdriver.DeadlockDetected
		}

		var counted []string
		c.metrics.CommitFailures = &countingCounter{onAdd: func(labels []string) {
			counted = append(counted, labels...)
		}}

		err := c.Commit(ctx, testBlock(13))

		require.Error(t, err)
		require.Zero(t, calls.Load(), "a cancelled caller must not have the block committed")
		require.ErrorIs(t, err, context.Canceled,
			"the reason the caller stopped must stay matchable")
		// The class, not the error, is what Commit promises its caller and what the
		// metrics catalog promises an operator. Without the escalation this returns
		// classRetry, which both say never happens.
		require.Equal(t, classDegrade, classify(err),
			"an abandoned block must not report as retryable")
		// A cancelled context is a deliberate stop, not a channel that cannot
		// commit, so it must not raise the alert commit_failures drives.
		require.Empty(t, counted, "an abandoned block must not be counted as a failure")
	})

	t.Run("counts a deterministic failure returned while the context ends", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		c := newRetryCommitter(t)
		c.commitBlock = func(context.Context, *common.Block) error {
			cancel() // the node shuts down while the block fails for good
			return errors.Wrap(fdriver.ErrConfigRejected, "failed updating membership service")
		}

		var counted []string
		c.metrics.CommitFailures = &countingCounter{onAdd: func(labels []string) {
			counted = append(counted, labels...)
		}}

		err := c.Commit(ctx, testBlock(17))

		require.ErrorIs(t, err, fdriver.ErrConfigRejected)
		require.Equal(t, []string{failureClassLabel, classDegrade.String()}, counted)
	})

	t.Run("abandoning mid-retry keeps the last failure readable", func(t *testing.T) {
		t.Parallel()
		// The point of routing both exits through abandon: giving up must not throw
		// away what had been going wrong. Without it the caller sees only
		// "context canceled" and loses the fault that caused the retrying.
		ctx, cancel := context.WithCancel(t.Context())
		c := newRetryCommitter(t)
		c.commitBlock = func(context.Context, *common.Block) error {
			cancel() // the caller gives up while a transient fault is in progress
			return errors.Wrap(dbdriver.DeadlockDetected, "vault contention")
		}

		err := c.Commit(ctx, testBlock(25))

		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled, "why we stopped")
		require.ErrorIs(t, err, dbdriver.DeadlockDetected, "what was failing")
		require.ErrorContains(t, err, "vault contention")
		// The escalation must survive carrying a retryable cause: classify checks
		// ErrRetriesExhausted first so the deadlock cannot pull this back to retry.
		require.Equal(t, classDegrade, classify(err))
	})

	t.Run("recovers when a lost database connection comes back", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		c := newRetryCommitter(t)
		c.commitBlock = func(context.Context, *common.Block) error {
			if calls.Add(1) <= 2 {
				return errors.Wrapf(dbdriver.ErrConnectionLost, "failed writing txids")
			}
			return nil
		}

		require.NoError(t, c.Commit(t.Context(), testBlock(31)))
		require.Equal(t, int32(3), calls.Load())
	})

	t.Run("keeps retrying a lost connection beyond the budget", func(t *testing.T) {
		t.Parallel()
		// An outage longer than the budget must not stop the channel: a missing
		// database says nothing about the block.
		const outage = testCommitRetries * 3
		var calls atomic.Int32
		c := withRetrySleep(newRetryCommitter(t))
		c.commitBlock = func(context.Context, *common.Block) error {
			if calls.Add(1) <= outage {
				return errors.Wrap(dbdriver.ErrConnectionLost, "dial tcp: connect: connection refused")
			}
			return nil
		}

		// NoError also rules out ErrRetriesExhausted: the budget must not apply.
		require.NoError(t, c.Commit(t.Context(), testBlock(33)))
		require.Equal(t, int32(outage+1), calls.Load())
	})

	t.Run("abandons a lost connection that never clears once the context ends", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		var calls atomic.Int32
		c := withRetrySleep(newRetryCommitter(t))
		c.commitBlock = func(context.Context, *common.Block) error {
			if calls.Add(1) == testCommitRetries*3 {
				cancel()
			}
			return errors.Wrap(dbdriver.ErrConnectionLost, "connection refused")
		}

		err := c.Commit(ctx, testBlock(35))

		require.ErrorIs(t, err, ErrRetriesExhausted)
		require.ErrorIs(t, err, dbdriver.ErrConnectionLost)
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, classDegrade, classify(err))
		require.Equal(t, int32(testCommitRetries*3), calls.Load())
	})

	t.Run("contention after a lost connection keeps its own budget", func(t *testing.T) {
		t.Parallel()
		// Attempts lost to the connection do not use up the contention budget,
		// so a block that loses its connection and then hits contention still
		// gets every contention retry.
		var calls atomic.Int32
		c := withRetrySleep(newRetryCommitter(t))
		c.commitBlock = func(context.Context, *common.Block) error {
			if calls.Add(1) <= testCommitRetries {
				return dbdriver.ErrConnectionLost
			}
			return dbdriver.DeadlockDetected
		}

		err := c.Commit(t.Context(), testBlock(37))

		require.ErrorIs(t, err, ErrRetriesExhausted)
		require.ErrorIs(t, err, dbdriver.DeadlockDetected)
		require.Equal(t, int32(2*testCommitRetries+1), calls.Load())
	})

	t.Run("a zero budget commits once", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		c := newRetryCommitter(t)
		c.ChannelConfig = &fake.ChannelConfig{IDValue: "testChannel", Retries: 0}
		c.commitBlock = func(context.Context, *common.Block) error {
			calls.Add(1)
			return dbdriver.DeadlockDetected
		}

		require.Error(t, c.Commit(t.Context(), testBlock(15)))
		require.Equal(t, int32(1), calls.Load())
	})
}
