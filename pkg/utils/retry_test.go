/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

func TestRetryRunner_SucceedsImmediately(t *testing.T) {
	t.Parallel()
	runner := NewRetryRunner(3, 0, false)
	calls := 0
	err := runner.Run(func() error {
		calls++
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

func TestRetryRunner_RetriesAndSucceeds(t *testing.T) {
	t.Parallel()
	runner := NewRetryRunner(5, 0, false)
	calls := 0
	err := runner.Run(func() error {
		calls++
		if calls < 3 {
			return errors.New("not yet")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, calls)
}

func TestRetryRunner_ExceedsMaxRetries(t *testing.T) {
	t.Parallel()
	runner := NewRetryRunner(3, 0, false)
	sentinelErr := errors.New("always fails")
	err := runner.Run(func() error {
		return sentinelErr
	})
	require.Error(t, err)
	require.ErrorIs(t, err, sentinelErr)
}

func TestRetryRunner_ExceedsMaxRetries_NoErrors(t *testing.T) {
	t.Parallel()
	// RunWithErrors returning (false, nil) means retry but no error to collect.
	// After maxTimes, ErrMaxRetriesExceeded is returned.
	runner := NewRetryRunner(3, 0, false)
	err := runner.RunWithErrors(func() (bool, error) {
		return false, nil
	})
	require.ErrorIs(t, err, ErrMaxRetriesExceeded)
}

func TestRetryRunner_RunWithErrors_TerminatesWithError(t *testing.T) {
	t.Parallel()
	runner := NewRetryRunner(3, 0, false)
	sentinelErr := errors.New("terminal error")
	err := runner.RunWithErrors(func() (bool, error) {
		return true, sentinelErr
	})
	require.ErrorIs(t, err, sentinelErr)
}

func TestRetryRunner_RunWithErrors_TerminatesSuccessfully(t *testing.T) {
	t.Parallel()
	runner := NewRetryRunner(3, 0, false)
	err := runner.RunWithErrors(func() (bool, error) {
		return true, nil
	})
	require.NoError(t, err)
}

func TestRetryRunner_Infinitely(t *testing.T) {
	t.Parallel()
	runner := NewRetryRunner(Infinitely, 0, false)
	calls := 0
	err := runner.Run(func() error {
		calls++
		if calls < 5 {
			return errors.New("not yet")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 5, calls)
}

func TestRetryRunner_ExponentialBackoff(t *testing.T) {
	t.Parallel()
	runner := NewRetryRunner(3, time.Millisecond, true)
	calls := 0
	err := runner.Run(func() error {
		calls++
		if calls < 3 {
			return errors.New("not yet")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, calls)
}

func TestTypedRetryRunner_ReturnsValue(t *testing.T) {
	t.Parallel()
	runner := NewTypedRetryRunner[string](3, 0, false)
	calls := 0
	val, err := runner.Run(func() (string, error) {
		calls++
		if calls < 2 {
			return "", errors.New("not yet")
		}
		return "success", nil
	})
	require.NoError(t, err)
	require.Equal(t, "success", val)
}

func TestTypedRetryRunner_ExceedsMax(t *testing.T) {
	t.Parallel()
	runner := NewTypedRetryRunner[int](2, 0, false)
	val, err := runner.Run(func() (int, error) {
		return 0, errors.New("always fails")
	})
	require.Error(t, err)
	require.Equal(t, 0, val)
}

func TestProbabilisticRetryRunner_Succeeds(t *testing.T) {
	t.Parallel()
	runner := NewProbabilisticRetryRunner(3, 1, false)
	calls := 0
	err := runner.Run(func() error {
		calls++
		if calls < 2 {
			return errors.New("not yet")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

func TestProbabilisticRetryRunner_ExceedsMax(t *testing.T) {
	t.Parallel()
	runner := NewProbabilisticRetryRunner(2, 1, false)
	err := runner.Run(func() error {
		return errors.New("always fails")
	})
	require.Error(t, err)
}

func TestRetryRunner_ExponentialBackoffDoesNotOverflow(t *testing.T) {
	t.Parallel()
	runner := NewRetryRunner(Infinitely, time.Millisecond, true)
	for range 100 {
		require.Positive(t, runner.NextDelay())
	}
}

func TestRunWithErrors_NoSleepAfterLastAttempt(t *testing.T) {
	t.Parallel()
	start := time.Now()
	err := NewRetryRunner(1, time.Second, false).Run(func() error { return errors.New("fail") })
	require.Error(t, err)
	require.Less(t, time.Since(start), 500*time.Millisecond)
}

func TestBackoff_CappedAndJittered(t *testing.T) {
	t.Parallel()
	runner := NewBackoffRetryRunner(Infinitely, 10*time.Millisecond, 100*time.Millisecond)
	for range 20 {
		d := runner.NextDelay()
		require.GreaterOrEqual(t, d, time.Duration(0))
		require.LessOrEqual(t, d, 100*time.Millisecond)
	}

	// A growing ceiling yields distinct delays even without jitter, so pin
	// the ceiling: any variation can then only come from the jitter.
	fixed := NewBackoffRetryRunner(Infinitely, 5*time.Millisecond, 5*time.Millisecond)
	seen := map[time.Duration]bool{}
	for range 50 {
		seen[fixed.NextDelay()] = true
	}
	require.Greater(t, len(seen), 10, "delays are not jittered")
}

func TestBackoff_MaxDelayBelowInitialIsRaised(t *testing.T) {
	t.Parallel()
	// The cap is raised to initial (10ms), so delays keep being drawn from
	// [0, 10ms) instead of collapsing to the 1ms passed as maxDelay.
	runner := NewBackoffRetryRunner(Infinitely, 10*time.Millisecond, time.Millisecond)
	runner.NextDelay()
	var above bool
	for range 20 {
		d := runner.NextDelay()
		require.GreaterOrEqual(t, d, time.Duration(0))
		require.Less(t, d, 10*time.Millisecond)
		above = above || d > time.Millisecond
	}
	require.True(t, above, "delays collapsed below initial")
}

func TestBackoff_NoOverflowAndZeroInitial(t *testing.T) {
	t.Parallel()
	runner := NewBackoffRetryRunner(Infinitely, time.Millisecond, math.MaxInt64)
	for range 100 {
		require.GreaterOrEqual(t, runner.NextDelay(), time.Duration(0))
	}
	require.Zero(t, NewBackoffRetryRunner(1, 0, time.Second).NextDelay())
}

func TestRunContext_NoSleepAfterLastAttempt(t *testing.T) {
	t.Parallel()
	start := time.Now()
	// A non-jittered runner makes the skipped 1s wait deterministic.
	err := NewRetryRunner(1, time.Second, false).
		RunContext(t.Context(), func() error { return errors.New("fail") })
	require.ErrorIs(t, err, ErrMaxRetriesExceeded)
	require.Less(t, time.Since(start), 500*time.Millisecond)
}

func TestRunContext_StopsOnCancel(t *testing.T) {
	t.Parallel()
	runner := NewBackoffRetryRunner(Infinitely, 10*time.Second, time.Minute)
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(50*time.Millisecond, cancel)

	start := time.Now()
	err := runner.RunContext(ctx, func() error { return errors.New("down") })

	require.Less(t, time.Since(start), time.Second, "wait was not interrupted")
	require.ErrorIs(t, err, context.Canceled)
}

func TestRunContext_CancelledBeforeFirstAttempt(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls := 0
	err := NewBackoffRetryRunner(3, 0, 0).RunContext(ctx, func() error {
		calls++
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls)
}

func TestRunContext_KeepsLastErrorOnly(t *testing.T) {
	t.Parallel()
	calls := 0
	err := NewBackoffRetryRunner(3, 0, 0).RunContext(t.Context(), func() error {
		calls++
		return errors.Errorf("attempt %d", calls)
	})
	require.Equal(t, 3, calls)
	require.ErrorIs(t, err, ErrMaxRetriesExceeded)
	require.ErrorContains(t, err, "attempt 3")
	require.NotContains(t, err.Error(), "attempt 1")
	require.NotContains(t, err.Error(), "attempt 2")
}

func TestRunContext_SucceedsAfterRetries(t *testing.T) {
	t.Parallel()
	calls := 0
	runner := NewBackoffRetryRunner(Infinitely, time.Millisecond, 5*time.Millisecond)
	err := runner.RunContext(t.Context(), func() error {
		calls++
		if calls < 5 {
			return errors.New("not yet")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 5, calls)
}
