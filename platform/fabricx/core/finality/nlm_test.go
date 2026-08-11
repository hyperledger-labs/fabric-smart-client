/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality/mock"
)

// To re-generate the mock/ run "go generate" directive
//go:generate counterfeiter -o mock/notifier_client.go --fake-name Notifier_OpenNotificationStreamClient github.com/hyperledger/fabric-x-common/api/committerpb.Notifier_OpenNotificationStreamClient
//go:generate counterfeiter -o mock/notifier_grpc_client.go --fake-name NotifierClient github.com/hyperledger/fabric-x-common/api/committerpb.NotifierClient

const (
	tick               = 10 * time.Millisecond
	timeout            = 1 * time.Second
	shortWait          = 100 * time.Millisecond
	testRequestTimeout = 30 * time.Second
)

// mockListener is a helper to verify callbacks
type mockListener struct {
	txID    string
	status  int
	calls   int
	wgCount int // how many callbacks wg is waiting for; see OnStatus
	wg      sync.WaitGroup
	lock    sync.RWMutex
}

func (m *mockListener) OnStatus(ctx context.Context, txID string, status int, errMsg string) {
	m.lock.Lock()
	m.txID = txID
	m.status = status
	m.calls++
	expected := m.calls <= m.wgCount
	m.lock.Unlock()

	// Only count down for callbacks the test asked to wait for. A listener still
	// registered when listen() exits is now also settled with Unknown on teardown,
	// and tests that never expected a callback must not see a negative WaitGroup
	// counter for it. Assertions on txID/status/calls are unaffected.
	if expected {
		m.wg.Done()
	}
}

// expect declares how many callbacks the test will wait on via wg.
func (m *mockListener) expect(n int) {
	m.lock.Lock()
	m.wgCount += n
	m.lock.Unlock()
	m.wg.Add(n)
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

func setupTest(tb testing.TB) (*notificationListenerManager, *mock.Notifier_OpenNotificationStreamClient) {
	tb.Helper()

	fakeStream := &mock.Notifier_OpenNotificationStreamClient{}
	fakeClient := &mock.NotifierClient{}

	// Configure the client to return our fake stream
	fakeClient.OpenNotificationStreamStub = func(c context.Context, opts ...grpc.CallOption) (committerpb.Notifier_OpenNotificationStreamClient, error) {
		fakeStream.ContextReturns(c)
		return fakeStream, nil
	}

	// listenerTTL is deliberately left zero here, which disables local expiry, so
	// the sweeper stays inert for every test that does not opt in.
	nlm := &notificationListenerManager{
		notifyClient:   fakeClient,
		requestQueue:   make(chan *committerpb.NotificationRequest),
		responseQueue:  make(chan *committerpb.NotificationResponse),
		handlers:       make(map[driver.TxID]*handlerEntry),
		requestTimeout: testRequestTimeout,
	}

	return nlm, fakeStream
}

// seedHandlers registers listeners directly, bypassing AddFinalityListener, to
// isolate dispatch and sweep logic. It keeps the map's internal shape in one
// place. Note expiresAt is left zero, meaning "never expires": call setExpiry to
// make an entry sweep-eligible.
func seedHandlers(nlm *notificationListenerManager, txID string, listeners ...fabric.FinalityListener) {
	nlm.handlersMu.Lock()
	defer nlm.handlersMu.Unlock()
	nlm.handlers[txID] = &handlerEntry{listeners: listeners}
}

// listenersFor returns a snapshot of the listeners registered for txID, plus
// whether the entry exists.
func listenersFor(nlm *notificationListenerManager, txID string) ([]fabric.FinalityListener, bool) {
	nlm.handlersMu.RLock()
	defer nlm.handlersMu.RUnlock()
	entry, ok := nlm.handlers[txID]
	if !ok {
		return nil, false
	}
	return slices.Clone(entry.listeners), true
}

// expiryOf returns an entry's local deadline, plus whether the entry exists.
func expiryOf(nlm *notificationListenerManager, txID string) (time.Time, bool) {
	nlm.handlersMu.RLock()
	defer nlm.handlersMu.RUnlock()
	entry, ok := nlm.handlers[txID]
	if !ok {
		return time.Time{}, false
	}
	return entry.expiresAt, true
}

// setExpiry overrides an entry's local deadline so tests can control expiry.
func setExpiry(nlm *notificationListenerManager, txID string, at time.Time) {
	nlm.handlersMu.Lock()
	defer nlm.handlersMu.Unlock()
	if entry, ok := nlm.handlers[txID]; ok {
		entry.expiresAt = at
	}
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
	t.Parallel()
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
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				nlm, fakeStream := setupTest(t)
				ctx := t.Context()
				// setup a mock listener expectation
				ml := &mockListener{}
				ml.expect(1)

				// manually inject into the map to isolate the Receive/Dispatch logic
				seedHandlers(nlm, tc.txID, ml)

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
		ml.expect(1)
		seedHandlers(nlm, targetTxID, ml)

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
		require.Equal(t, durationpb.New(testRequestTimeout), req.GetTimeout(), "outbound request must carry requestTimeout so the committer can reply early")
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
		handlers, exists := listenersFor(nlm, targetTxID)
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
		handlers, exists := listenersFor(nlm, targetTxID)

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

	t.Run("Joining_Listener_Inherits_The_Existing_Deadline", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_deadline_inherited"
		nlm, fakeStream := setupTest(t)
		nlm.listenerTTL = time.Hour // long, so nothing expires during the test
		ctx := t.Context()
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		require.NoError(t, nlm.AddFinalityListener(targetTxID, &mockListener{}))
		first, ok := expiryOf(nlm, targetTxID)
		require.True(t, ok, "entry should exist after the first registration")
		require.False(t, first.IsZero(), "a deadline must be stamped when listenerTTL is set")

		// Registering a second listener for the same txID must not push the deadline
		// out. Otherwise a txID that keeps attracting listeners could stay in the map
		// indefinitely -- the exact unbounded growth this change exists to prevent.
		time.Sleep(2 * tick) // ensure a later Now() would produce a different deadline
		require.NoError(t, nlm.AddFinalityListener(targetTxID, &mockListener{}))

		second, ok := expiryOf(nlm, targetTxID)
		require.True(t, ok, "entry should still exist")
		require.Equal(t, first, second, "a joining listener must inherit the deadline, not extend it")

		handlers, _ := listenersFor(nlm, targetTxID)
		require.Len(t, handlers, 2, "both listeners should be registered")
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

	t.Run("AddFinalityListener_Empty_TxID_Fails", func(t *testing.T) {
		t.Parallel()
		nlm, fakeStream := setupTest(t)
		ctx := t.Context()
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		// an empty txID must be rejected: no notification could ever match it, so
		// the entry would never be removed
		err := nlm.AddFinalityListener("", &mockListener{})

		require.Error(t, err)
		require.EqualError(t, err, "tx id must be not empty",
			"message must match the generic driver's, so both drivers agree")

		nlm.handlersMu.RLock()
		_, exists := nlm.handlers[""]
		nlm.handlersMu.RUnlock()
		require.False(t, exists, "No handler entry should be created for an empty txID")

		// and no subscription should have been sent for it
		time.Sleep(shortWait)
		require.Equal(t, 0, fakeStream.SendCallCount(), "Empty txID must not trigger a Send")
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

	t.Run("Shutdown_Settles_Pending_Listeners_With_Unknown", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_pending_at_shutdown"
		nlm, fakeStream := setupTest(t)
		// handlerTimeout must be non-zero here: invokeHandler derives the handler
		// context from it, and a zero timeout would expire before OnStatus runs.
		nlm.handlerTimeout = config.DefaultHandlerTimeout

		ctx, cancel := context.WithCancel(context.Background())
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		// A delayedListener respects context cancellation, which a real listener
		// does too. That matters here: teardown runs after the errgroup context is
		// already cancelled, so handing listeners that context would deliver
		// nothing. A mockListener would not notice, because it ignores ctx.
		ml := &delayedListener{delay: tick}
		ml.expect(1)
		seedHandlers(nlm, targetTxID, ml)

		listenErr := make(chan error, 1)
		go func() { listenErr <- nlm.listen(ctx) }()
		time.Sleep(shortWait)

		// The stream dies with a listener still pending. Nothing can ever notify it,
		// so teardown must settle it rather than drop it silently.
		cancel()

		select {
		case <-listenErr:
		case <-time.After(timeout):
			t.Fatal("listen() did not return after context cancellation")
		}

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			txID, status := ml.getStatus()
			assert.Equal(collect, targetTxID, txID,
				"a listener pending at teardown must still be invoked")
			assert.Equal(collect, fdriver.Unknown, status,
				"teardown cannot know the outcome, so it reports Unknown")
		}, timeout, tick, "timeout waiting for OnStatus on stream teardown")

		_, exists := listenersFor(nlm, targetTxID)
		require.False(t, exists, "handlers map must be empty after teardown")
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

		setupListeners, setupExists := listenersFor(nlm, targetTxID)
		require.True(t, setupExists, "Setup: entry should exist")
		require.Len(t, setupListeners, 2, "Setup: Expected 2 listeners")

		err := nlm.RemoveFinalityListener(targetTxID, ml1)
		require.NoError(t, err, "RemoveFinalityListener for ml1 should succeed")

		// map entry must still exist and contain only ml2
		handlers, exists := listenersFor(nlm, targetTxID)

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

	t.Run("Removing_A_Listener_Preserves_The_Deadline", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_deadline_preserved"
		nlm, fakeStream := setupTest(t)
		nlm.listenerTTL = time.Hour // long, so nothing expires during the test
		ctx := t.Context()
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		runManager(t, nlm)

		ml1, ml2 := &mockListener{}, &mockListener{}
		require.NoError(t, nlm.AddFinalityListener(targetTxID, ml1))
		require.NoError(t, nlm.AddFinalityListener(targetTxID, ml2))

		before, ok := expiryOf(nlm, targetTxID)
		require.True(t, ok)
		require.False(t, before.IsZero(), "a deadline must be stamped when listenerTTL is set")

		// Removing one listener must not reset or extend the deadline for the ones
		// still waiting: their entry should expire when it always would have.
		require.NoError(t, nlm.RemoveFinalityListener(targetTxID, ml1))

		after, ok := expiryOf(nlm, targetTxID)
		require.True(t, ok, "entry should survive while ml2 is still registered")
		require.Equal(t, before, after, "removing a listener must not change the deadline")

		handlers, _ := listenersFor(nlm, targetTxID)
		require.Len(t, handlers, 1)
		require.Equal(t, ml2, handlers[0])
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
		setupListeners, setupExists := listenersFor(nlm, targetTxID)
		require.True(t, setupExists, "Setup: entry should exist")
		require.Len(t, setupListeners, 2, "Setup: Expected 2 listeners")

		// attempt to remove ml3 which was never added
		err := nlm.RemoveFinalityListener(targetTxID, ml3Nonexistent)
		require.NoError(t, err, "Attempt to remove non-existent listener should return nil")

		// map must be unchanged (still 2 listeners)
		handlers, exists := listenersFor(nlm, targetTxID)

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
		require.Empty(t, nlm.handlers, "Handler map should remain empty")
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

		seedHandlers(nlm, targetTxID, slowListener)

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
		fastML.expect(1)

		// slowListener completes, but takes a while (still within timeout)
		slowML := &delayedListener{delay: 100 * time.Millisecond}
		slowML.expect(1)

		// stuckListener never returns (exceeds timeout)
		stuckCalled := make(chan struct{})
		stuckListener := &blockingListener{
			block:    make(chan struct{}), // never closed
			onCalled: stuckCalled,
		}

		seedHandlers(nlm, targetTxID, fastML, slowML, stuckListener)

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
		normalML.expect(1)

		seedHandlers(nlm, leakyTxID, leakyListener)
		seedHandlers(nlm, normalTxID, normalML)

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

const (
	testTTL   = 50 * time.Millisecond
	testSweep = 10 * time.Millisecond
)

// setupSweepTest builds a manager with local expiry enabled and a Recv that
// blocks, so the sweeper is the only thing touching the handlers map.
func setupSweepTest(tb testing.TB) (*notificationListenerManager, *mock.Notifier_OpenNotificationStreamClient) {
	tb.Helper()
	nlm, fakeStream := setupTest(tb)
	nlm.listenerTTL = testTTL
	nlm.sweepInterval = testSweep
	// setupTest leaves handlerTimeout zero, which would hand every listener an
	// already-expired context; set it so the sweeper's callbacks are realistic.
	nlm.handlerTimeout = config.DefaultHandlerTimeout
	return nlm, fakeStream
}

// blockingRecv makes Recv park until the context is done, so no notification ever
// arrives and only expiry can settle a listener.
func blockingRecv(ctx context.Context, fakeStream *mock.Notifier_OpenNotificationStreamClient) {
	fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
}

func TestSweepExpired(t *testing.T) {
	t.Parallel()

	t.Run("Deadline_Stamped_By_AddFinalityListener", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_production_deadline"
		nlm, fakeStream := setupSweepTest(t)
		blockingRecv(t.Context(), fakeStream)

		runManager(t, nlm)

		ml := &mockListener{}
		ml.expect(1)
		// Register through the real API and set NO deadline by hand: the entry must
		// expire purely because AddFinalityListener stamped it. This is what proves
		// the leak is actually fixed in production, not just in the sweeper.
		require.NoError(t, nlm.AddFinalityListener(targetTxID, ml))

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			txID, status := ml.getStatus()
			assert.Equal(collect, targetTxID, txID)
			assert.Equal(collect, fdriver.Unknown, status)
		}, timeout, tick, "listener registered via AddFinalityListener must be settled by expiry")

		_, exists := listenersFor(nlm, targetTxID)
		require.False(t, exists, "expired entry must be removed from the map")
	})

	t.Run("Expired_Entry_Is_Settled_With_Unknown", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_expired"
		nlm, fakeStream := setupSweepTest(t)
		blockingRecv(t.Context(), fakeStream)

		ml := &mockListener{}
		ml.expect(1)
		seedHandlers(nlm, targetTxID, ml)
		setExpiry(nlm, targetTxID, time.Now().Add(-time.Second)) // already overdue

		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			txID, status := ml.getStatus()
			assert.Equal(collect, targetTxID, txID)
			assert.Equal(collect, fdriver.Unknown, status,
				"expiry reports Unknown, matching the committer's own timeout path")
		}, timeout, tick, "timeout waiting for the sweeper to settle the listener")

		_, exists := listenersFor(nlm, targetTxID)
		require.False(t, exists, "expired entry must be removed from the map")
	})

	t.Run("Unexpired_Entry_Survives", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_not_yet_due"
		nlm, fakeStream := setupSweepTest(t)
		blockingRecv(t.Context(), fakeStream)

		ml := &mockListener{} // no wg.Add: OnStatus must NOT be called
		seedHandlers(nlm, targetTxID, ml)
		setExpiry(nlm, targetTxID, time.Now().Add(time.Hour))

		runManager(t, nlm)
		time.Sleep(shortWait) // many sweep intervals

		_, exists := listenersFor(nlm, targetTxID)
		require.True(t, exists, "an entry whose deadline has not passed must not be swept")
	})

	t.Run("Zero_TTL_Disables_Expiry", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_expiry_disabled"
		nlm, fakeStream := setupTest(t) // listenerTTL stays zero
		nlm.sweepInterval = testSweep   // but tick fast, so the guard is what stops us
		blockingRecv(t.Context(), fakeStream)

		ml := &mockListener{} // no wg.Add: OnStatus must NOT be called
		seedHandlers(nlm, targetTxID, ml)
		setExpiry(nlm, targetTxID, time.Now().Add(-time.Second)) // overdue on purpose

		runManager(t, nlm)
		time.Sleep(shortWait) // many sweep intervals

		_, exists := listenersFor(nlm, targetTxID)
		require.True(t, exists, "listenerTTL == 0 must disable expiry even for an overdue entry")
	})

	t.Run("Notification_Wins_Without_Double_Invoke", func(t *testing.T) {
		t.Parallel()
		const targetTxID = "tx_no_double_settle"
		nlm, fakeStream := setupSweepTest(t)
		ctx := t.Context()

		resp := &committerpb.NotificationResponse{
			TxStatusEvents: []*committerpb.TxStatus{{
				Ref:    &committerpb.TxRef{TxId: targetTxID},
				Status: committerpb.Status_COMMITTED,
			}},
		}
		var sent atomic.Bool
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			if !sent.Swap(true) {
				return resp, nil
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}

		// wg.Add(1) is the trap: a second OnStatus panics with "negative WaitGroup
		// counter", which is exactly the double-settle we must never allow.
		ml := &mockListener{}
		ml.expect(1)
		seedHandlers(nlm, targetTxID, ml)
		// A real deadline matters here. seedHandlers leaves expiresAt zero, and the
		// sweeper skips zero-expiry entries, so without this the sweeper would never
		// be a contender and the trap would be armed against nothing.
		setExpiry(nlm, targetTxID, time.Now().Add(testTTL))

		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			_, status := ml.getStatus()
			assert.Equal(collect, fdriver.Valid, status, "the notification should win")
		}, timeout, tick, "timeout waiting for the notification to settle the listener")

		// let several sweep intervals pass beyond the deadline
		time.Sleep(4 * testTTL)

		_, exists := listenersFor(nlm, targetTxID)
		require.False(t, exists, "entry was removed by the notification")
	})
}
