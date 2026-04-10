/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/stretchr/testify/require"
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
