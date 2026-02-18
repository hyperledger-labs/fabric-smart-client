/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality/mock"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// To re-generate the mock/ run "go generate" directive
//go:generate counterfeiter -o mock/notifier_client.go github.com/hyperledger/fabric-x-common/api/committerpb.Notifier_OpenNotificationStreamClient
//go:generate counterfeiter -o mock/notifier_grpc_client.go github.com/hyperledger/fabric-x-common/api/committerpb.NotifierClient

const (
	tick      = 10 * time.Millisecond
	timeout   = 1 * time.Second
	shortWait = 100 * time.Millisecond
)

// mockListener is a helper to verify callbacks
type mockListener struct {
	txID   string
	status int
	wg     sync.WaitGroup
	lock   sync.RWMutex
}

func (m *mockListener) OnStatus(ctx context.Context, txID string, status int, errMsg string) {
	m.lock.Lock()
	m.txID = txID
	m.status = status
	m.lock.Unlock()
	m.wg.Done()
}

// getStatus is a helper to safely read the state for use in EventuallyWithT
func (m *mockListener) getStatus() (string, int) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.txID, m.status
}

// blockingListener simulates a handler that blocks forever (ignores context).
// Used to test timeout detection and goroutine leak resilience.
type blockingListener struct {
	block    chan struct{} // close this to unblock; leave open to simulate a stuck handler
	onCalled chan struct{} // closed when OnStatus is entered, so tests can synchronize
}

func (b *blockingListener) OnStatus(_ context.Context, _ string, _ int, _ string) {
	if b.onCalled != nil {
		select {
		case <-b.onCalled:
			// already closed
		default:
			close(b.onCalled)
		}
	}
	<-b.block // block until unblocked or leak
}

// delayedListener completes after a configurable delay.
// Embeds mockListener so getStatus() works for assertions.
type delayedListener struct {
	mockListener
	delay time.Duration
}

func (d *delayedListener) OnStatus(ctx context.Context, txID string, status int, errMsg string) {
	select {
	case <-time.After(d.delay):
	case <-ctx.Done():
		return
	}
	d.mockListener.OnStatus(ctx, txID, status, errMsg)
}

func setupTest(tb testing.TB) (*notificationListenerManager, *mock.FakeNotifier_OpenNotificationStreamClient) {
	tb.Helper()

	fakeStream := &mock.FakeNotifier_OpenNotificationStreamClient{}
	fakeClient := &mock.FakeNotifierClient{}

	// Configure the client to return our fake stream
	fakeClient.OpenNotificationStreamStub = func(c context.Context, opts ...grpc.CallOption) (committerpb.Notifier_OpenNotificationStreamClient, error) {
		fakeStream.ContextReturns(c)
		return fakeStream, nil
	}

	nlm := &notificationListenerManager{
		notifyClient:  fakeClient,
		requestQueue:  make(chan *committerpb.NotificationRequest),
		responseQueue: make(chan *committerpb.NotificationResponse),
		handlers:      make(map[driver.TxID][]fabric.FinalityListener),
	}

	return nlm, fakeStream
}

// runManager starts the manager asynchronously and ensures cleanup on test completion.
func runManager(t *testing.T, nlm *notificationListenerManager) {
	t.Helper()

	// Start listen() in a goroutine (it's blocking)
	listenErr := make(chan error, 1)
	go func() {
		listenErr <- nlm.listen(t.Context())
	}()

	// Test contect is used so context is canceled just before Cleanup-registered functions are called.
	t.Cleanup(func() {
		// Wait for listen() to return
		err := <-listenErr
		// listen() should return context.Canceled on graceful shutdown
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("listen() returned unexpected error: %v", err)
		}
	})

	// Give goroutines a moment to start
	time.Sleep(shortWait)
}

func TestNotificationListenerManager(t *testing.T) {
	t.Run("Listen_Shutdown_Lifecycle_And_Cleanup", func(t *testing.T) {
		t.Parallel()
		nlm, fakeStream := setupTest(t)
		ctx := t.Context()
		// Mock Recv to block until context is done
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		// Start the manager
		runManager(t, nlm)

		// The test passes if runManager completes without panic/error during cleanup,
		// proving the Listen/Shutdown logic is sound.
	})

	t.Run("Receive_And_Dispatch_HappyPath", func(t *testing.T) {
		t.Parallel()

		table := []struct {
			name           string
			txID           string
			serverStatus   committerpb.Status
			expectedStatus int
		}{
			{
				name:           "Committed Transaction",
				txID:           "tx_valid",
				serverStatus:   committerpb.Status_COMMITTED,
				expectedStatus: fdriver.Valid,
			},
			{
				name:           "Invalid Transaction",
				txID:           "tx_invalid",
				serverStatus:   committerpb.Status_ABORTED_SIGNATURE_INVALID,
				expectedStatus: fdriver.Invalid,
			},
			{
				name:           "Unknown Transaction",
				txID:           "tx_unknown",
				serverStatus:   committerpb.Status_STATUS_UNSPECIFIED,
				expectedStatus: fdriver.Unknown,
			},
		}

		for _, tc := range table {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				nlm, fakeStream := setupTest(t)
				ctx := t.Context()
				// setup a mock listener expectation
				ml := &mockListener{}
				ml.wg.Add(1)

				// manually inject into the map to isolate the Receive/Dispatch logic
				nlm.handlers[tc.txID] = []fabric.FinalityListener{ml}

				// prepare the incoming gRPC message
				resp := &committerpb.NotificationResponse{
					TxStatusEvents: []*committerpb.TxStatus{
						{
							Ref:    &committerpb.TxRef{TxId: tc.txID},
							Status: tc.serverStatus,
						},
					},
				}

				// mock Recv to return data once then block
				var sent atomic.Bool
				fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
					if !sent.Swap(true) {
						return resp, nil
					}
					// simulate stream remaining open but idle
					<-ctx.Done()
					return nil, ctx.Err()
				}

				// run manager
				runManager(t, nlm)

				require.EventuallyWithT(t, func(collect *assert.CollectT) {
					txID, status := ml.getStatus()
					assert.Equal(collect, tc.txID, txID, "TxID should match expected value")
					assert.Equal(collect, tc.expectedStatus, status, "Status should match expected value")
				}, timeout, tick, "Timeout waiting for OnStatus callback with TxID %s", tc.txID)

				// verify handler was deleted (crucial cleanup check)
				nlm.handlersMu.RLock()
				_, exists := nlm.handlers[tc.txID]
				nlm.handlersMu.RUnlock()
				require.False(t, exists, "Handler should be removed after notification dispatch.")
			})
		}
	})

	t.Run("Receive_And_Dispatch_Handles_Timeout", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_timeout"
		nlm, fakeStream := setupTest(t)
		ctx := t.Context()
		ml := &mockListener{}
		ml.wg.Add(1)
		nlm.handlers[targetTxID] = []fabric.FinalityListener{ml}

		// prepare response with a TimeoutTxId
		resp := &committerpb.NotificationResponse{
			TimeoutTxIds: []string{targetTxID},
		}

		var sent atomic.Bool
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			if !sent.Swap(true) {
				return resp, nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			txID, status := ml.getStatus()
			assert.Equal(collect, targetTxID, txID, "TxID should match expected value after timeout")
			assert.Equal(collect, fdriver.Unknown, status, "Status should be Unknown (timeout) after dispatch")
		}, timeout, tick, "timeout waiting for OnStatus callback from timeout response")

		nlm.handlersMu.RLock()
		_, exists := nlm.handlers[targetTxID]
		nlm.handlersMu.RUnlock()
		require.False(t, exists, "Handler should be removed after notification dispatch.")
	})

	t.Run("AddFinalityListener_Triggers_Send", func(t *testing.T) {
		t.Parallel()
		nlm, fakeStream := setupTest(t)
		ctx := t.Context()
		// mock Recv to simply block so it doesn't interfere
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		// start the manager
		runManager(t, nlm)

		// add a listener
		ml := &mockListener{}
		err := nlm.AddFinalityListener("tx_send_check", ml)
		require.NoError(t, err)

		// verify Send was called
		require.Eventually(t, func() bool {
			return fakeStream.SendCallCount() == 1
		}, timeout, tick)

		req := fakeStream.SendArgsForCall(0)
		require.NotNil(t, req.GetTxStatusRequest())
		require.Contains(t, req.GetTxStatusRequest().GetTxIds(), "tx_send_check")
	})

	t.Run("AddFinalityListener_Duplicate_Is_Rejected", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_duplicate_listener"
		nlm, fakeStream := setupTest(t)
		ctx := t.Context()
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		ml := &mockListener{}

		// 1. Add the first listener (should trigger a Send)
		err := nlm.AddFinalityListener(targetTxID, ml)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			return fakeStream.SendCallCount() == 1
		}, timeout, tick, "First AddFinalityListener call did not trigger Send")

		// 2. Add the SAME listener instance again (should be rejected internally)
		err = nlm.AddFinalityListener(targetTxID, ml)
		require.NoError(t, err, "Duplicate AddFinalityListener call should return nil.")

		time.Sleep(shortWait)

		// verify Send was *not* called a second time.
		require.Equal(t, 1, fakeStream.SendCallCount(), "Duplicate AddFinalityListener call should NOT trigger a second Send")

		// verify only one handler was registered internally.
		nlm.handlersMu.RLock()
		handlers, exists := nlm.handlers[targetTxID]
		nlm.handlersMu.RUnlock()
		require.True(t, exists, "Handler list should exist after first registration")
		require.Len(t, handlers, 1, "There should be exactly ONE registered handler (the duplicate was rejected)")
		require.Equal(t, ml, handlers[0], "The registered handler must be the original instance (ml)")
	})

	t.Run("AddFinalityListener_Multiple_Unique_Are_Allowed", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_multiple_unique"
		nlm, fakeStream := setupTest(t)
		ctx := t.Context()
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		ml1 := &mockListener{txID: "1"}
		ml2 := &mockListener{txID: "2"}

		// 1. Add ml1 (Should trigger Send)
		err := nlm.AddFinalityListener(targetTxID, ml1)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			return fakeStream.SendCallCount() == 1
		}, timeout, tick, "First AddFinalityListener call did not trigger Send")

		// 2. Add ml2 (Should NOT trigger a second Send)
		err = nlm.AddFinalityListener(targetTxID, ml2)
		require.NoError(t, err)

		time.Sleep(shortWait)

		// verify Send was *not* called a second time.
		require.Equal(t, 1, fakeStream.SendCallCount(), "Second unique listener should NOT trigger a second Send")

		// verify BOTH unique listeners were registered internally.
		nlm.handlersMu.RLock()
		handlers, exists := nlm.handlers[targetTxID]
		nlm.handlersMu.RUnlock()

		require.True(t, exists, "Handler list should exist")
		require.Len(t, handlers, 2, "There should be exactly TWO registered handlers")

		// Check both listeners are present
		found1, found2 := false, false
		for _, h := range handlers {
			if h == ml1 {
				found1 = true
			}
			if h == ml2 {
				found2 = true
			}
		}
		require.True(t, found1 && found2, "Both unique listeners (ml1 and ml2) must be present in the handler list.")
	})

	t.Run("AddFinalityListener_Nil_Listener_Fails", func(t *testing.T) {
		t.Parallel()
		nlm, fakeStream := setupTest(t)
		ctx := t.Context()
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		// try to add a nil listener
		err := nlm.AddFinalityListener("tx_nil_check", nil)

		// assert that the function returned the specific error
		require.Error(t, err)
		require.EqualError(t, err, "listener nil", "Should return 'listener nil' error for a nil listener")

		nlm.handlersMu.RLock()
		_, exists := nlm.handlers["tx_nil_check"]
		nlm.handlersMu.RUnlock()
		require.False(t, exists, "Handler should not be added to the map for a nil listener")
	})

	t.Run("Shutdown_Graceful_Exit", func(t *testing.T) {
		t.Parallel()
		nlm, fakeStream := setupTest(t)
		ctx, cancel := context.WithCancel(context.Background())
		// mock Recv to block indefinitely on context
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		// Start listen() in a goroutine
		listenErr := make(chan error, 1)
		go func() {
			listenErr <- nlm.listen(ctx)
		}()

		// wait briefly to ensure all goroutines are up and blocking
		time.Sleep(shortWait)

		// Cancel the context (equivalent to Shutdown)
		cancel()

		// Wait for listen() to return
		select {
		case err := <-listenErr:
			// Should return context.Canceled on graceful shutdown
			require.True(t, errors.Is(err, context.Canceled), "listen() should return context.Canceled on graceful shutdown")
		case <-time.After(timeout):
			t.Fatal("listen() did not return after context cancellation within timeout")
		}
	})

	t.Run("Stream_Error_Handling", func(t *testing.T) {
		t.Parallel()
		nlm, fakeStream := setupTest(t)
		ctx := t.Context()

		expectedErr := errors.New("stream broken")
		fakeStream.RecvReturns(nil, expectedErr)

		// Start listen() in a goroutine
		listenErr := make(chan error, 1)
		go func() {
			listenErr <- nlm.listen(ctx)
		}()

		// listen() should return the error from Recv()
		select {
		case err := <-listenErr:
			require.Error(t, err, "listen() should return an error when stream breaks")
			// The error could be the original error or wrapped
		case <-time.After(timeout):
			t.Fatal("listen() did not return after stream error within timeout")
		}
	})

	t.Run("Remove_Single_Listener_Cleans_Up_Map", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_remove_single"
		nlm, fakeStream := setupTest(t)
		ctx := t.Context()

		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		ml := &mockListener{}

		require.NoError(t, nlm.AddFinalityListener(targetTxID, ml), "Setup: failed to add listener")

		err := nlm.RemoveFinalityListener(targetTxID, ml)
		require.NoError(t, err, "RemoveFinalityListener should succeed")

		// map entry must be deleted
		nlm.handlersMu.RLock()
		_, exists := nlm.handlers[targetTxID]
		nlm.handlersMu.RUnlock()
		require.False(t, exists, "Map entry should be deleted after the last listener is removed")
	})

	t.Run("Remove_One_Of_Multiple_Listeners_Keeps_Others", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_remove_one_of_two"
		nlm, fakeStream := setupTest(t)
		ctx := t.Context()

		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		ml1 := &mockListener{}
		ml2 := &mockListener{}

		require.NoError(t, nlm.AddFinalityListener(targetTxID, ml1), "Setup: failed to add ml1")
		require.NoError(t, nlm.AddFinalityListener(targetTxID, ml2), "Setup: failed to add ml2")

		nlm.handlersMu.RLock()
		require.Len(t, nlm.handlers[targetTxID], 2, "Setup: Expected 2 listeners")
		nlm.handlersMu.RUnlock()

		err := nlm.RemoveFinalityListener(targetTxID, ml1)
		require.NoError(t, err, "RemoveFinalityListener for ml1 should succeed")

		// map entry must still exist and contain only ml2
		nlm.handlersMu.RLock()
		handlers, exists := nlm.handlers[targetTxID]
		nlm.handlersMu.RUnlock()

		require.True(t, exists, "Map entry should still exist")
		require.Len(t, handlers, 1, "Expected 1 listener remaining (ml2)")
		require.Equal(t, ml2, handlers[0], "The remaining listener must be ml2")

		// remove the last listener (ml2)
		err = nlm.RemoveFinalityListener(targetTxID, ml2)
		require.NoError(t, err, "RemoveFinalityListener for ml2 should succeed")

		nlm.handlersMu.RLock()
		_, exists = nlm.handlers[targetTxID]
		nlm.handlersMu.RUnlock()
		require.False(t, exists, "Map entry should be deleted after ml2 is removed")
	})

	t.Run("Remove_NonExistent_Listener", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_remove_nonexistent"
		nlm, fakeStream := setupTest(t)
		ctx := t.Context()

		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		ml1 := &mockListener{}
		ml2 := &mockListener{}
		ml3Nonexistent := &mockListener{}

		require.NoError(t, nlm.AddFinalityListener(targetTxID, ml1), "Setup: failed to add ml1")
		require.NoError(t, nlm.AddFinalityListener(targetTxID, ml2), "Setup: failed to add ml2")

		// assert initial state
		nlm.handlersMu.RLock()
		require.Len(t, nlm.handlers[targetTxID], 2, "Setup: Expected 2 listeners")
		nlm.handlersMu.RUnlock()

		// attempt to remove ml3 which was never added
		err := nlm.RemoveFinalityListener(targetTxID, ml3Nonexistent)
		require.NoError(t, err, "Attempt to remove non-existent listener should return nil")

		// map must be unchanged (still 2 listeners)
		nlm.handlersMu.RLock()
		handlers, exists := nlm.handlers[targetTxID]
		nlm.handlersMu.RUnlock()

		require.True(t, exists, "Map entry should still exist")
		require.Len(t, handlers, 2, "The number of handlers should not change")

		require.Contains(t, handlers, ml1)
		require.Contains(t, handlers, ml2)
	})

	t.Run("Remove_Listener_From_NonExistent_TxID", func(t *testing.T) {
		t.Parallel()
		nlm, fakeStream := setupTest(t)
		ctx := t.Context()

		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		ml := &mockListener{}

		// remove a listener for a TxID that was never added
		err := nlm.RemoveFinalityListener("tx_does_not_exist", ml)
		require.NoError(t, err, "Attempt to remove listener for non-existent TxID should return nil")

		nlm.handlersMu.RLock()
		require.Len(t, nlm.handlers, 0, "Handler map should remain empty")
		nlm.handlersMu.RUnlock()
	})

	t.Run("Remove_Nil_Listener_Fails", func(t *testing.T) {
		t.Parallel()
		nlm, fakeStream := setupTest(t)
		ctx := t.Context()

		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		// try to remove a nil listener
		err := nlm.RemoveFinalityListener("tx_nil_check", nil)

		require.Error(t, err)
		require.EqualError(t, err, "listener nil", "Should return 'listener nil' error for a nil listener")
	})

	t.Run("Handler_Timeout_Detection", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_handler_timeout"
		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = 200 * time.Millisecond // short timeout for test speed
		ctx := t.Context()

		// slowListener blocks longer than the handler timeout
		slowCalled := make(chan struct{})
		slowListener := &blockingListener{
			block:    make(chan struct{}), // never closed, simulates a stuck handler
			onCalled: slowCalled,
		}

		nlm.handlers[targetTxID] = []fabric.FinalityListener{slowListener}

		resp := &committerpb.NotificationResponse{
			TxStatusEvents: []*committerpb.TxStatus{
				{
					Ref:    &committerpb.TxRef{TxId: targetTxID},
					Status: committerpb.Status_COMMITTED,
				},
			},
		}

		var sent atomic.Bool
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			if !sent.Swap(true) {
				return resp, nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		// Wait for the handler to be invoked
		select {
		case <-slowCalled:
			// Good, the handler was called
		case <-time.After(timeout):
			t.Fatal("slow handler was never invoked")
		}

		// The handler is still blocked, but the dispatcher should have moved on.
		// Verify the handler entry was removed from the map (dispatch cleanup
		// happens before the goroutine timeout fires).
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			nlm.handlersMu.RLock()
			_, exists := nlm.handlers[targetTxID]
			nlm.handlersMu.RUnlock()
			assert.False(collect, exists,
				"Handler should be removed from map even though it timed out")
		}, timeout, tick)
	})

	t.Run("Multiple_Handlers_Mixed_Speeds", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_mixed_speeds"
		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = 500 * time.Millisecond
		ctx := t.Context()

		// fastListener completes immediately
		fastML := &mockListener{}
		fastML.wg.Add(1)

		// slowListener completes, but takes a while (still within timeout)
		slowML := &delayedListener{delay: 100 * time.Millisecond}
		slowML.wg.Add(1)

		// stuckListener never returns (exceeds timeout)
		stuckCalled := make(chan struct{})
		stuckListener := &blockingListener{
			block:    make(chan struct{}), // never closed
			onCalled: stuckCalled,
		}

		nlm.handlers[targetTxID] = []fabric.FinalityListener{fastML, slowML, stuckListener}

		resp := &committerpb.NotificationResponse{
			TxStatusEvents: []*committerpb.TxStatus{
				{
					Ref:    &committerpb.TxRef{TxId: targetTxID},
					Status: committerpb.Status_COMMITTED,
				},
			},
		}

		var sent atomic.Bool
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			if !sent.Swap(true) {
				return resp, nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		// fastListener should complete quickly
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			txID, status := fastML.getStatus()
			assert.Equal(collect, targetTxID, txID)
			assert.Equal(collect, fdriver.Valid, status)
		}, timeout, tick, "fast listener should be notified")

		// slowListener should also complete (within timeout)
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			txID, status := slowML.getStatus()
			assert.Equal(collect, targetTxID, txID)
			assert.Equal(collect, fdriver.Valid, status)
		}, timeout, tick, "slow listener should be notified")

		// stuckListener was called but will time out
		select {
		case <-stuckCalled:
		case <-time.After(timeout):
			t.Fatal("stuck handler was never invoked")
		}

		// Handler map should be cleaned up regardless of stuck handler
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			nlm.handlersMu.RLock()
			_, exists := nlm.handlers[targetTxID]
			nlm.handlersMu.RUnlock()
			assert.False(collect, exists,
				"Handler map entry should be removed after dispatch")
		}, timeout, tick)
	})

	t.Run("Goroutine_Leak_Dispatcher_Not_Blocked", func(t *testing.T) {
		// Verifies that a handler ignoring context cancellation and never
		// returning does NOT block the dispatcher from processing subsequent
		// notifications.
		t.Parallel()
		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = 200 * time.Millisecond
		ctx := t.Context()

		const leakyTxID = "tx_leaky"
		const normalTxID = "tx_normal"

		// leakyListener ignores context it blocks forever
		leakyCalled := make(chan struct{})
		leakyListener := &blockingListener{
			block:    make(chan struct{}), // never closed
			onCalled: leakyCalled,
		}

		// normalListener completes promptly
		normalML := &mockListener{}
		normalML.wg.Add(1)

		nlm.handlers[leakyTxID] = []fabric.FinalityListener{leakyListener}
		nlm.handlers[normalTxID] = []fabric.FinalityListener{normalML}

		// First response triggers the leaky handler,
		// second triggers the normal one.
		leakyResp := &committerpb.NotificationResponse{
			TxStatusEvents: []*committerpb.TxStatus{
				{
					Ref:    &committerpb.TxRef{TxId: leakyTxID},
					Status: committerpb.Status_COMMITTED,
				},
			},
		}
		normalResp := &committerpb.NotificationResponse{
			TxStatusEvents: []*committerpb.TxStatus{
				{
					Ref:    &committerpb.TxRef{TxId: normalTxID},
					Status: committerpb.Status_COMMITTED,
				},
			},
		}

		callCount := atomic.Int32{}
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			n := callCount.Add(1)
			switch n {
			case 1:
				return leakyResp, nil
			case 2:
				return normalResp, nil
			default:
				<-ctx.Done()
				return nil, ctx.Err()
			}
		}

		runManager(t, nlm)

		// Wait for the leaky handler to be invoked
		select {
		case <-leakyCalled:
		case <-time.After(timeout):
			t.Fatal("leaky handler was never invoked")
		}

		// Critical assertion: normalListener must still get notified even
		// though leakyListener is stuck forever. This proves the dispatcher
		// loop is not blocked by a misbehaving handler.
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			txID, status := normalML.getStatus()
			assert.Equal(collect, normalTxID, txID)
			assert.Equal(collect, fdriver.Valid, status)
		}, timeout, tick,
			"normal listener must be notified despite leaky handler")
	})
}
