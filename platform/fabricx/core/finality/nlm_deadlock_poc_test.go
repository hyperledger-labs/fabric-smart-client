/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// TestAddFinalityListenerRecoversAfterStreamFailure proves the fix for the
// permanent deadlock this manager used to suffer once the underlying gRPC
// notification stream failed (e.g. a malicious/compromised Notifier endpoint
// drops or errors the stream -- trivially triggerable by closing the
// connection, and also naturally triggered by a normal server restart):
//
//  1. listen() exits (all three of its internal goroutines unwind via the
//     errgroup once Recv() returns an error).
//  2. Nothing ever drains n.requestQueue again, since the "sender" goroutine
//     that used to read from it is gone.
//  3. The *next* call to AddFinalityListener for any new txID now detects,
//     via streamCtx, that the stream that would have serviced its send is
//     dead, and returns an error promptly instead of blocking forever while
//     holding handlersMu.
//  4. Because the lock is released promptly, every subsequent call to
//     AddFinalityListener/RemoveFinalityListener, for ANY txID on this
//     manager, is never blocked behind it.
//
// Before the fix, a single dropped/failed gRPC connection to the Notifier
// service permanently bricked finality tracking for the affected
// network/channel. Now, AddFinalityListener fails fast with an error,
// leaving the manager (and, crucially, handlersMu) usable.
func TestAddFinalityListenerRecoversAfterStreamFailure(t *testing.T) {
	nlm, fakeStream := setupTest(t)

	// Simulate the Notifier stream failing immediately, as a malicious or
	// restarting committer would.
	fakeStream.RecvReturns(nil, errors.New("stream broken"))

	listenErr := make(chan error, 1)
	go func() {
		listenErr <- nlm.listen(t.Context())
	}()

	// Wait for listen() to fully unwind (its goroutines exit because Recv
	// failed), confirming the sender goroutine that used to drain
	// requestQueue is now gone.
	select {
	case <-listenErr:
		// listen() has exited; its internal goroutines (including the one
		// that used to drain requestQueue) are gone.
	case <-time.After(timeout):
		t.Fatal("listen() did not exit after simulated stream failure")
	}

	// The manager is now in the "stream died" state that any real deployment
	// reaches after a single dropped connection. A legitimate caller
	// attempting to track finality for a brand-new transaction must not get
	// stuck forever.
	done := make(chan error, 1)
	go func() {
		done <- nlm.AddFinalityListener("victim-tx", &mockListener{})
	}()

	select {
	case err := <-done:
		require.Error(t, err, "AddFinalityListener should fail fast once the notification stream is dead")
		require.Contains(t, err.Error(), "notification stream unavailable")
	case <-time.After(timeout):
		t.Fatal("AddFinalityListener did not return promptly after stream failure (deadlock reproduced)")
	}

	// The failed registration must not have left a stale entry behind.
	nlm.handlersMu.RLock()
	_, exists := nlm.handlers["victim-tx"]
	nlm.handlersMu.RUnlock()
	require.False(t, exists, "a failed AddFinalityListener call must not leave a dangling handler entry")

	// Prove the lock is genuinely free: RemoveFinalityListener for a
	// completely unrelated txID -- which only needs the same mutex, not the
	// dead stream -- must also complete promptly.
	removeDone := make(chan struct{})
	go func() {
		_ = nlm.RemoveFinalityListener("unrelated-tx", &mockListener{})
		close(removeDone)
	}()

	select {
	case <-removeDone:
		// Expected: the mutex was never left held, so this returns promptly.
	case <-time.After(timeout):
		t.Fatal("RemoveFinalityListener did not return promptly -- handlersMu is still stuck")
	}
}
