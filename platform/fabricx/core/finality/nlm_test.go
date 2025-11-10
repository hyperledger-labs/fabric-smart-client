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
)

// To re-generate the mock/ run "go generate" directive
//go:generate counterfeiter -o mock/notifier_client.go github.com/hyperledger/fabric-x-committer/api/protonotify.Notifier_OpenNotificationStreamClient

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

func setupTest(tb testing.TB) (*notificationListenerManager, *mock.FakeNotifier_OpenNotificationStreamClient, context.Context) {
	tb.Helper()

	ctx := tb.Context()

	fakeStream := &mock.FakeNotifier_OpenNotificationStreamClient{}
	fakeStream.ContextReturns(ctx)

	nlm := &notificationListenerManager{
		notifyStream:  fakeStream,
		requestQueue:  make(chan *protonotify.NotificationRequest, 1),
		responseQueue: make(chan *protonotify.NotificationResponse, 1),
		handlers:      make(map[driver.TxID][]fabric.FinalityListener),
	}

	return nlm, fakeStream, ctx
}

func TestNotificationListenerManager(t *testing.T) {
	t.Run("Listen happy path (Receive & Dispatch)", func(t *testing.T) {
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
				nlm, fakeStream, ctx := setupTest(t)

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

				// run Listen
				go func() {
					_ = nlm.Listen(ctx)
				}()

				require.EventuallyWithT(t, func(collect *assert.CollectT) {
					txID, status := ml.getStatus()
					assert.Equal(collect, tc.txID, txID, "TxID should match expected value")
					assert.Equal(collect, tc.expectedStatus, status, "Status should match expected value")
				}, timeout, tick, "Timeout waiting for OnStatus callback with TxID %s", tc.txID)

				// verify handler was deleted (crucial cleanup check)
				nlm.lock.RLock()
				_, exists := nlm.handlers[tc.txID]
				nlm.lock.RUnlock()
				require.False(t, exists, "Handler should be removed after notification dispatch.")

			})
		}
	})

	t.Run("Listen_Handles_Timeout_Response", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_timeout"
		nlm, fakeStream, ctx := setupTest(t)

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

		go func() {
			_ = nlm.Listen(ctx)
		}()

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			txID, status := ml.getStatus()
			assert.Equal(collect, targetTxID, txID, "TxID should match expected value after timeout")
			assert.Equal(collect, fdriver.Unknown, status, "Status should be Unknown (timeout) after dispatch")
		}, timeout, tick, "timeout waiting for OnStatus callback from timeout response")

		nlm.lock.RLock()
		_, exists := nlm.handlers[targetTxID]
		nlm.lock.RUnlock()
		require.False(t, exists, "Handler should be removed after notification dispatch.")
	})

	t.Run("AddFinalityListener triggers Send", func(t *testing.T) {
		t.Parallel()
		nlm, fakeStream, ctx := setupTest(t)

		// mock Recv to simply block so it doesn't interfere
		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		// start the manager
		go func() {
			_ = nlm.Listen(ctx)
		}()

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
		nlm, fakeStream, ctx := setupTest(t)

		// mock Recv to simply block so it doesn't interfere
		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		// start the manager
		go func() {
			_ = nlm.Listen(ctx)
		}()

		// The single listener instance used for both calls
		ml := &mockListener{}

		// 1. Add the first listener (should trigger a Send)
		err := nlm.AddFinalityListener(targetTxID, ml)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			return fakeStream.SendCallCount() == 1
		}, timeout, tick, "First AddFinalityListener call did not trigger Send")

		// 2. Add the SAME listener instance again (should be rejected internally)
		err = nlm.AddFinalityListener(targetTxID, ml)

		// Assert: The new logic returns nil (no error) and skips registration.
		require.NoError(t, err, "Duplicate AddFinalityListener call should return nil.")

		// wait a small duration
		time.Sleep(shortWait)

		// verify Send was *not* called a second time.
		require.Equal(t, 1, fakeStream.SendCallCount(), "Duplicate AddFinalityListener call should NOT trigger a second Send")

		// verify only one handler was registered internally.
		nlm.lock.RLock()
		handlers, exists := nlm.handlers[targetTxID]
		nlm.lock.RUnlock()
		require.True(t, exists, "Handler list should exist after first registration")
		require.Len(t, handlers, 1, "There should be exactly ONE registered handler (the duplicate was rejected)")
		require.Equal(t, ml, handlers[0], "The registered handler must be the original instance (ml)")
	})

	t.Run("AddFinalityListener_Multiple_Unique_Are_Allowed", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_multiple_unique"
		nlm, fakeStream, ctx := setupTest(t)

		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		go func() {
			_ = nlm.Listen(ctx)
		}()

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
		nlm.lock.RLock()
		handlers, exists := nlm.handlers[targetTxID]
		nlm.lock.RUnlock()

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
		nlm, _, _ := setupTest(t)

		// try to add a nil listener
		err := nlm.AddFinalityListener("tx_nil_check", nil)

		// assert that the function returned the specific error
		require.Error(t, err)
		require.EqualError(t, err, "listener nil", "Should return 'listener nil' error for a nil listener")

		nlm.lock.RLock()
		_, exists := nlm.handlers["tx_nil_check"]
		nlm.lock.RUnlock()
		require.False(t, exists, "Handler should not be added to the map for a nil listener")
	})

	t.Run("Listen_Context_Cancellation_Graceful_Exit", func(t *testing.T) {
		t.Parallel()
		nlm, fakeStream, ctx := setupTest(t)

		cancelable_ctx, cancel := context.WithCancel(ctx)

		// mock Recv and Send to block indefinitely to ensure the context is the source of exit
		fakeStream.RecvStub = func() (*protonotify.NotificationResponse, error) {
			<-cancelable_ctx.Done()
			return nil, cancelable_ctx.Err()
		}
		fakeStream.SendStub = func(*protonotify.NotificationRequest) error {
			<-cancelable_ctx.Done()
			return cancelable_ctx.Err()
		}

		// start the manager
		errs := make(chan error, 1)
		go func() {
			errs <- nlm.Listen(cancelable_ctx)
		}()

		// wait briefly to ensure all goroutines are up and blocking
		time.Sleep(shortWait)

		// cancel the context, which should cause all three goroutines (Receiver, Sender, Dispatcher) to exit gracefully
		cancel()

		// verify Listen exits with a context error
		select {
		case err := <-errs:
			// the error group will return the first non-nil error from one of the goroutines.
			// it should be context.Canceled or similar
			require.Contains(t, err.Error(), "context canceled", "Listen should exit gracefully on context cancellation")
		case <-time.After(timeout):
			t.Fatal("timeout waiting for Listen to exit after context cancellation")
		}
	})

	t.Run("Stream Error Handling", func(t *testing.T) {
		t.Parallel()
		nlm, fakeStream, ctx := setupTest(t)

		expectedErr := errors.New("stream broken")
		fakeStream.RecvReturns(nil, expectedErr)

		// listen should return the error immediately
		err := nlm.Listen(ctx)
		require.ErrorIs(t, err, expectedErr)
	})

	t.Run("Remove_Single_Listener_Cleans_Up_Map", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_remove_single"
		nlm, _, _ := setupTest(t)

		ml := &mockListener{}

		require.NoError(t, nlm.AddFinalityListener(targetTxID, ml), "Setup: failed to add listener")

		err := nlm.RemoveFinalityListener(targetTxID, ml)
		require.NoError(t, err, "RemoveFinalityListener should succeed")

		// map entry must be deleted
		nlm.lock.RLock()
		_, exists := nlm.handlers[targetTxID]
		nlm.lock.RUnlock()
		require.False(t, exists, "Map entry should be deleted after the last listener is removed")
	})

	t.Run("Remove_One_Of_Multiple_Listeners_Keeps_Others", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_remove_one_of_two"
		nlm, _, _ := setupTest(t)

		ml1 := &mockListener{}
		ml2 := &mockListener{}

		require.NoError(t, nlm.AddFinalityListener(targetTxID, ml1), "Setup: failed to add ml1")
		require.NoError(t, nlm.AddFinalityListener(targetTxID, ml2), "Setup: failed to add ml2")

		nlm.lock.RLock()
		require.Len(t, nlm.handlers[targetTxID], 2, "Setup: Expected 2 listeners")
		nlm.lock.RUnlock()

		err := nlm.RemoveFinalityListener(targetTxID, ml1)
		require.NoError(t, err, "RemoveFinalityListener for ml1 should succeed")

		// map entry must still exist and contain only ml2
		nlm.lock.RLock()
		handlers, exists := nlm.handlers[targetTxID]
		nlm.lock.RUnlock()

		require.True(t, exists, "Map entry should still exist")
		require.Len(t, handlers, 1, "Expected 1 listener remaining (ml2)")
		require.Equal(t, ml2, handlers[0], "The remaining listener must be ml2")

		// remove the last listener (ml2)
		err = nlm.RemoveFinalityListener(targetTxID, ml2)
		require.NoError(t, err, "RemoveFinalityListener for ml2 should succeed")

		nlm.lock.RLock()
		_, exists = nlm.handlers[targetTxID]
		nlm.lock.RUnlock()
		require.False(t, exists, "Map entry should be deleted after ml2 is removed")
	})

	t.Run("Remove_NonExistent_Listener", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_remove_nonexistent"
		nlm, _, _ := setupTest(t)

		ml1 := &mockListener{}
		ml2 := &mockListener{}
		ml3Nonexistent := &mockListener{}

		require.NoError(t, nlm.AddFinalityListener(targetTxID, ml1), "Setup: failed to add ml1")
		require.NoError(t, nlm.AddFinalityListener(targetTxID, ml2), "Setup: failed to add ml2")

		// assert initial state
		nlm.lock.RLock()
		initialHandlers := nlm.handlers[targetTxID]
		require.Len(t, initialHandlers, 2, "Setup: Expected 2 listeners")
		nlm.lock.RUnlock()

		// attempt to remove ml3 which was never added
		err := nlm.RemoveFinalityListener(targetTxID, ml3Nonexistent)
		require.NoError(t, err, "Attempt to remove non-existent listener should return nil")

		// map must be unchanged (still 2 listeners)
		nlm.lock.RLock()
		handlers, exists := nlm.handlers[targetTxID]
		nlm.lock.RUnlock()

		require.True(t, exists, "Map entry should still exist")
		require.Len(t, handlers, 2, "The number of handlers should not change")

		require.Contains(t, handlers, ml1)
		require.Contains(t, handlers, ml2)
	})

	t.Run("Remove_Listener_From_NonExistent_TxID", func(t *testing.T) {
		t.Parallel()
		nlm, _, _ := setupTest(t)

		ml := &mockListener{}

		// remove a listener for a TxID that was never added
		err := nlm.RemoveFinalityListener("tx_does_not_exist", ml)
		require.NoError(t, err, "Attempt to remove listener for non-existent TxID should return nil")

		nlm.lock.RLock()
		require.Len(t, nlm.handlers, 0, "Handler map should remain empty")
		nlm.lock.RUnlock()
	})

	t.Run("Remove_Nil_Listener_Fails", func(t *testing.T) {
		t.Parallel()
		nlm, _, _ := setupTest(t)

		// try to remove a nil listener
		err := nlm.RemoveFinalityListener("tx_nil_check", nil)

		require.Error(t, err)
		require.EqualError(t, err, "listener nil", "Should return 'listener nil' error for a nil listener")
	})

}
