/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package assert

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pkgerrors "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

func TestRetrySucceedsImmediately(t *testing.T) {
	t.Parallel()

	calls := 0
	err := Retry(2, 0, func() error {
		calls++
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

func TestRetryEventuallySucceeds(t *testing.T) {
	t.Parallel()

	calls := 0
	err := Retry(4, 0, func() error {
		calls++
		if calls < 3 {
			return pkgerrors.New("not yet")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, calls)
}

func TestRetryReturnsLastErrorAfterAttempts(t *testing.T) {
	t.Parallel()

	sentinelErr := errors.New("still no luck")
	err := Retry(2, time.Nanosecond, func() error {
		return sentinelErr
	})

	require.Error(t, err)
	require.ErrorContains(t, err, "no luck after 2 attempts")
	require.ErrorContains(t, err, sentinelErr.Error())
}

func TestEventuallyWithRetry(t *testing.T) {
	t.Parallel()

	calls := 0
	EventuallyWithRetry(t, 3, time.Nanosecond, func() error {
		calls++
		if calls < 2 {
			return pkgerrors.New("retry")
		}
		return nil
	})
	require.Equal(t, 2, calls)
}
