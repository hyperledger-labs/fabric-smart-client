/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	stderrors "errors"
	"sync/atomic"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want failureClass
	}{
		{"nil is none", nil, classNone},
		{"deadlock is retryable", dbdriver.DeadlockDetected, classRetry},
		{"sql busy is retryable", dbdriver.SqlBusy, classRetry},
		{"deadline exceeded is retryable", context.DeadlineExceeded, classRetry},
		{"canceled is retryable", context.Canceled, classRetry},
		{"config rejected degrades", fdriver.ErrConfigRejected, classDegrade},
		{"fatal is fatal", ErrFatalCommit, classFatal},
		{"unknown error degrades", stderrors.New("boom"), classDegrade},
		// The committer wraps every error on its way up through commitTxs and
		// the handler, so classification has to survive wrapping or it would
		// only ever see classDegrade in production.
		{"wrapped deadlock is retryable", errors.Wrapf(dbdriver.DeadlockDetected, "failed committing tx"), classRetry},
		{"deeply wrapped deadlock is retryable", errors.Wrapf(errors.Wrapf(dbdriver.DeadlockDetected, "inner"), "outer"), classRetry},
		{"wrapped config rejection degrades", errors.Wrapf(fdriver.ErrConfigRejected, "failed updating membership"), classDegrade},
		{"wrapped fatal is fatal", errors.Wrapf(ErrFatalCommit, "vault unreconcilable"), classFatal},
		// Fatal outranks a retryable cause it happens to be wrapped with, so a
		// node that cannot trust its state never spins retrying instead.
		{"fatal outranks retryable", errors.Wrapf(ErrFatalCommit, "after %v", dbdriver.DeadlockDetected), classFatal},
		{"retries exhausted degrades", ErrRetriesExhausted, classDegrade},
		// An exhausted budget keeps the transient error that caused it reachable,
		// which is what makes the check order in classify load-bearing: testing
		// contention before exhaustion would classify this retryable again and
		// replay the block forever.
		{
			"exhausted outranks the transient cause it carries",
			errors.Join(ErrRetriesExhausted, dbdriver.DeadlockDetected),
			classDegrade,
		},
		{
			"exhausted outranks a wrapped transient cause",
			errors.Wrapf(errors.Join(ErrRetriesExhausted, errors.Wrapf(dbdriver.SqlBusy, "still busy")), "block [1] failed"),
			classDegrade,
		},
		// Fatal still outranks exhaustion, so a node that cannot trust its state
		// is never merely degraded because the failure happened to arrive on the
		// last retry.
		{
			"fatal outranks exhausted",
			errors.Join(ErrRetriesExhausted, ErrFatalCommit),
			classFatal,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, classify(tc.err), "class for [%v]", tc.err)
		})
	}
}

func TestFailureClassString(t *testing.T) {
	t.Parallel()

	require.Equal(t, "none", classNone.String())
	require.Equal(t, "retry", classRetry.String())
	require.Equal(t, "degrade", classDegrade.String())
	require.Equal(t, "fatal", classFatal.String())
	require.Equal(t, "unknown", failureClass(99).String())
}

// testCommitRetries is the retry budget these tests pin, independent of the
// configured default, so that a change to the default does not silently change
// what the retry tests assert.
const testCommitRetries = 3

// nowaitChannelConfig is a channel configuration whose only purpose is a zero
// retry delay, so the retry tests exercise the real code path without sleeping.
type nowaitChannelConfig struct {
	fdriver.ChannelConfig
}

func (*nowaitChannelConfig) DeliverySleepAfterFailure() time.Duration { return 0 }

// newRetryDelivery builds a Delivery wired for readBlocks only.
//
// metrics and channelConfig are populated as New would populate them, so the
// counter increments and the delay lookup run here rather than being guarded
// against nil in production code. NewMetrics substitutes a discarding provider
// for a nil one, so the counters cost nothing and need no registry.
func newRetryDelivery(callback func(context.Context, *cb.Block) (bool, error)) *Delivery {
	return &Delivery{
		bufferSize:    1,
		stop:          make(chan struct{}),
		commitRetries: testCommitRetries,
		callback:      callback,
		metrics:       NewMetrics(nil),
		channelConfig: &nowaitChannelConfig{},
	}
}

func blockChan(n uint64) chan blockResponse {
	ch := make(chan blockResponse, 1)
	ch <- blockResponse{block: &cb.Block{Header: &cb.BlockHeader{Number: n}}}
	return ch
}

// TestReadBlocksRetriesTransient is the regression test for issue #1731: a
// transient commit failure must not end the channel's block stream.
func TestReadBlocksRetriesTransient(t *testing.T) {
	t.Parallel()

	t.Run("recovers when a transient failure clears", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		d := newRetryDelivery(func(_ context.Context, _ *cb.Block) (bool, error) {
			// Fail twice with a wrapped deadlock, as the committer would, then
			// succeed and ask to stop so readBlocks returns.
			if calls.Add(1) <= 2 {
				return false, errors.Wrapf(dbdriver.DeadlockDetected, "failed committing tx")
			}
			return true, nil
		})

		d.readBlocks(blockChan(7))

		<-d.stop
		require.NoError(t, d.stopError(), "a cleared transient failure must not stop with an error")
		require.Equal(t, int32(3), calls.Load(), "block should be replayed until it commits")
	})

	t.Run("gives up after the retry budget and reports the failure", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		d := newRetryDelivery(func(_ context.Context, _ *cb.Block) (bool, error) {
			calls.Add(1)
			return false, errors.Wrapf(dbdriver.SqlBusy, "still busy")
		})

		d.readBlocks(blockChan(9))

		<-d.stop
		stopErr := d.stopError()
		// One initial attempt plus the budget: the retry is bounded, so a
		// permanent fault reaches a decision instead of spinning.
		require.Equal(t, int32(testCommitRetries+1), calls.Load())
		// The class is what an operator alerts on, so assert it rather than only
		// the attempt count. A fault that survived every retry must be reported
		// as a degrade: leaving it in the retry class would hide the exhausted
		// case from the alert written for it.
		require.ErrorIs(t, stopErr, ErrRetriesExhausted)
		require.Equal(t, classDegrade, classify(stopErr),
			"an exhausted retry budget must escalate out of classRetry")
		// Asserted with ErrorIs, not ErrorContains: the escalation has to keep the
		// cause matchable for a caller branching on it, and a substring check on
		// the message passes just as well when the cause has been flattened into
		// text and dropped from the chain.
		require.ErrorIs(t, stopErr, dbdriver.SqlBusy,
			"the original cause must stay machine-readable, not just render in the message")
	})

	t.Run("stops immediately on a deterministic failure", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		d := newRetryDelivery(func(_ context.Context, _ *cb.Block) (bool, error) {
			calls.Add(1)
			return false, errors.Wrapf(fdriver.ErrConfigRejected, "failed updating membership service")
		})

		d.readBlocks(blockChan(11))

		<-d.stop
		require.ErrorContains(t, d.stopError(), "failed updating membership service")
		require.Equal(t, int32(1), calls.Load(), "a deterministic failure must not be retried")
	})

	t.Run("abandons retries when the service is stopped", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		d := newRetryDelivery(func(_ context.Context, _ *cb.Block) (bool, error) {
			calls.Add(1)
			return false, dbdriver.DeadlockDetected
		})
		// A service already stopped must not commit the block at all, let alone
		// burn the whole retry budget on a node that is shutting down.
		d.Stop(stderrors.New("shutting down"))

		d.readBlocks(blockChan(13))

		require.Zero(t, calls.Load(), "a stopped service must not commit")
		require.ErrorContains(t, d.stopError(), "shutting down",
			"the original stop cause must not be overwritten")
	})
}

// TestReadBlocksFatal covers the fatal class reaching the installed handler
// rather than being silently swallowed or taking the process down itself.
func TestReadBlocksFatal(t *testing.T) {
	t.Parallel()

	t.Run("invokes the fatal handler", func(t *testing.T) {
		t.Parallel()
		var gotNetwork, gotChannel string
		var gotErr error
		d := newRetryDelivery(func(_ context.Context, _ *cb.Block) (bool, error) {
			return false, errors.Wrapf(ErrFatalCommit, "vault state unreconcilable")
		})
		d.NetworkName = "testNet"
		d.channel = "testChannel"
		d.WithFatalHandler(func(network, channel string, err error) {
			gotNetwork, gotChannel, gotErr = network, channel, err
		})

		d.readBlocks(blockChan(15))

		<-d.stop
		require.Equal(t, "testNet", gotNetwork)
		require.Equal(t, "testChannel", gotChannel)
		require.ErrorContains(t, gotErr, "vault state unreconcilable")
		require.ErrorContains(t, d.stopError(), "vault state unreconcilable")
	})

	t.Run("stops cleanly with no handler installed", func(t *testing.T) {
		t.Parallel()
		d := newRetryDelivery(func(_ context.Context, _ *cb.Block) (bool, error) {
			return false, ErrFatalCommit
		})

		// Must not panic: a Delivery never exits the process itself, so an
		// absent handler leaves the channel stopped and logged, nothing more.
		d.readBlocks(blockChan(17))

		<-d.stop
		require.ErrorIs(t, d.stopError(), ErrFatalCommit)
	})
}

// TestNewMetricsNilProvider covers a Delivery built without a metrics provider
// still counting without panicking, since the counters are a diagnostic rather
// than a precondition for delivering blocks.
func TestNewMetricsNilProvider(t *testing.T) {
	t.Parallel()

	m := NewMetrics(nil)
	require.NotNil(t, m)
	m.CommitRetries.Add(1)
	m.CommitFailures.With(failureClassLabel, classDegrade.String()).Add(1)
}
