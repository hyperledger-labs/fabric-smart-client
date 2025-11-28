/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	mock "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality/mock"
	"github.com/hyperledger/fabric-x-committer/api/protoblocktx"
	"github.com/hyperledger/fabric-x-committer/api/protonotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// To re-generate the mock/ run "go generate" directive
//go:generate counterfeiter -o mock/notifier_client.go github.com/hyperledger/fabric-x-committer/api/protonotify.Notifier_OpenNotificationStreamClient
//go:generate counterfeiter -o mock/notifier_grpc_client.go github.com/hyperledger/fabric-x-committer/api/protonotify.NotifierClient

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

func setupTest(tb testing.TB) (*notificationListenerManager, *mock.FakeNotifier_OpenNotificationStreamClient) {
	tb.Helper()

	fakeStream := &mock.FakeNotifier_OpenNotificationStreamClient{}
	fakeClient := &mock.FakeNotifierClient{}

	// Configure the client to return our fake stream
	fakeClient.OpenNotificationStreamStub = func(c context.Context, opts ...grpc.CallOption) (protonotify.Notifier_OpenNotificationStreamClient, error) {
		fakeStream.ContextReturns(c)
		return fakeStream, nil
	}

	nlm := &notificationListenerManager{
		notifyClient:  fakeClient,
		requestQueue:  make(chan *protonotify.NotificationRequest),
		responseQueue: make(chan *protonotify.NotificationResponse),
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
		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
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
			serverStatus   protoblocktx.Status
			expectedStatus int
		}{
			{
				name:           "Committed Transaction",
				txID:           "tx_valid",
				serverStatus:   protoblocktx.Status_COMMITTED,
				expectedStatus: fdriver.Valid,
			},
			{
				name:           "Invalid Transaction",
				txID:           "tx_invalid",
				serverStatus:   protoblocktx.Status_ABORTED_SIGNATURE_INVALID,
				expectedStatus: fdriver.Invalid,
			},
			{
				name:           "Unknown Transaction",
				txID:           "tx_unknown",
				serverStatus:   protoblocktx.Status_NOT_VALIDATED,
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
				resp := &protonotify.NotificationResponse{
					TxStatusEvents: []*protonotify.TxStatusEvent{
						{
							TxId: tc.txID,
							StatusWithHeight: &protoblocktx.StatusWithHeight{
								Code: tc.serverStatus,
							},
						},
					},
				}

				// mock Recv to return data once then block
				var sent atomic.Bool
				fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
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
		resp := &protonotify.NotificationResponse{
			TimeoutTxIds: []string{targetTxID},
		}

		var sent atomic.Bool
		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
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
		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
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
		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
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
		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
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
		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
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
		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
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

		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
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

		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
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

		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
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

		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
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

		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		// try to remove a nil listener
		err := nlm.RemoveFinalityListener("tx_nil_check", nil)

		require.Error(t, err)
		require.EqualError(t, err, "listener nil", "Should return 'listener nil' error for a nil listener")
	})
}
