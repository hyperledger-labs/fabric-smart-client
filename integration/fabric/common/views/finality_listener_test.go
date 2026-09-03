/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func TestFinalityListenerReportsExpectedStatus(t *testing.T) {
	t.Parallel()
	l := views.NewFinalityListener("tx1")
	l.OnStatus(context.Background(), "tx1", fdriver.Valid, "")
	require.NoError(t, l.Expect(context.Background(), fdriver.Valid, time.Second))
}

func TestFinalityListenerReportsUnexpectedStatus(t *testing.T) {
	t.Parallel()
	l := views.NewFinalityListener("tx1")
	l.OnStatus(context.Background(), "tx1", fdriver.Unknown, "committer did not answer")

	err := l.Expect(context.Background(), fdriver.Valid, time.Second)
	require.Error(t, err)
	// The message must name the transaction and carry the status message the committer
	// gave: that is the whole point of the type, so assert on it rather than on Error()
	// alone.
	assert.Contains(t, err.Error(), "tx1")
	assert.Contains(t, err.Error(), "committer did not answer")
}

func TestFinalityListenerIgnoresOtherTransactions(t *testing.T) {
	t.Parallel()
	l := views.NewFinalityListener("tx1")
	l.OnStatus(context.Background(), "tx2", fdriver.Valid, "")

	err := l.Expect(context.Background(), fdriver.Valid, 50*time.Millisecond)
	require.Error(t, err, "a status for another transaction must not settle this listener")
}

func TestFinalityListenerGivesUp(t *testing.T) {
	t.Parallel()

	// Both bounds end at the same select branch, so one test covers them: the timeout
	// elapsing, and a context cancelled well inside a timeout that would otherwise hold.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	for name, tc := range map[string]struct {
		ctx     context.Context
		timeout time.Duration
	}{
		"timeout elapses":   {context.Background(), 50 * time.Millisecond},
		"context cancelled": {cancelled, time.Minute},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := views.NewFinalityListener("tx1").Expect(tc.ctx, fdriver.Valid, tc.timeout)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "tx1")
		})
	}
}

func TestFinalityListenerFirstStatusWins(t *testing.T) {
	t.Parallel()
	l := views.NewFinalityListener("tx1")
	// A redelivery must neither panic nor overwrite the first report.
	l.OnStatus(context.Background(), "tx1", fdriver.Valid, "")
	l.OnStatus(context.Background(), "tx1", fdriver.Unknown, "")

	require.NoError(t, l.Expect(context.Background(), fdriver.Valid, time.Second))
}

func TestFinalityListenerOnStatusDoesNotBlock(t *testing.T) {
	t.Parallel()
	l := views.NewFinalityListener("tx1")

	// OnStatus MUST return promptly (platform/common/driver/committer.go). Nobody is
	// waiting on this listener, so a blocking implementation hangs here.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 3 {
			l.OnStatus(context.Background(), "tx1", fdriver.Valid, "")
		}
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("OnStatus blocked")
	}
}
