/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer/fake"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
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

func testBlock(n uint64) *common.Block {
	return &common.Block{Header: &common.BlockHeader{Number: n}}
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
				return errors.Wrapf(dbdriver.DeadlockDetected, "failed committing tx")
			}
			return nil
		}

		require.NoError(t, c.Commit(t.Context(), testBlock(7)))
		require.Equal(t, int32(3), calls.Load(), "the block should be committed again until it lands")
	})

	t.Run("gives up after the budget and escalates out of the retry class", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		c := newRetryCommitter(t)
		c.commitBlock = func(context.Context, *common.Block) error {
			calls.Add(1)
			return errors.Wrapf(dbdriver.SqlBusy, "still busy")
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
			return errors.Wrapf(fdriver.ErrConfigRejected, "failed updating membership service")
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

		require.Error(t, c.Commit(ctx, testBlock(13)))
		require.Zero(t, calls.Load(), "a cancelled caller must not have the block committed")
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
