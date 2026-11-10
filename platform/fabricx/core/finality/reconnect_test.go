/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate/connstatetest"
)

// startReconnecting runs nlm over fn with run(), the reconnect loop, and returns the
// first stream. Cancelling the returned function stops the manager and waits for it.
func startReconnecting(t *testing.T, nlm *notificationListenerManager, fn *fakeNotifier) (*fakeStream, context.CancelFunc) {
	t.Helper()
	nlm.newNotifyClient = staticClient(fn)
	if nlm.handlerTimeout == 0 {
		nlm.handlerTimeout = time.Second
	}
	// Short enough that a test's outage never outgrows fakeNotifier.next's wait.
	nlm.reconnectMaxDelay = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- nlm.run(ctx) }()
	stop := func() {
		cancel()
		select {
		case err := <-done:
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(timeout):
			t.Fatal("run did not return after its context ended")
		}
	}
	t.Cleanup(cancel)
	s := fn.next(t)
	// next returns once the stream is opened; wait until it is also attached, so that
	// registrations from here on are sent on it rather than kept for the next one.
	require.Eventually(t, func() bool { return nlm.streamCtx.Load() != nil }, timeout, tick)
	return s, stop
}

// countingListener records every OnStatus call.
type countingListener struct {
	calls   atomic.Int32
	status  atomic.Int32
	message atomic.Pointer[string]
}

func (l *countingListener) OnStatus(_ context.Context, _ driver.TxID, status int, message string) {
	l.status.Store(int32(status))
	l.message.Store(&message)
	l.calls.Add(1)
}

// switchResolver answers every txID with code and the message for it, both of which a
// test may change while listeners are pending. calls counts Statuses invocations.
type switchResolver struct {
	code, calls atomic.Int32
	message     atomic.Pointer[string]
}

func (r *switchResolver) Statuses(_ context.Context, txIDs ...driver.TxID) ([]driver.TxValidationStatus[fdriver.ValidationCode], error) {
	r.calls.Add(1)
	var message string
	if m := r.message.Load(); m != nil {
		message = *m
	}
	out := make([]driver.TxValidationStatus[fdriver.ValidationCode], len(txIDs))
	for i, id := range txIDs {
		out[i] = driver.TxValidationStatus[fdriver.ValidationCode]{TxID: id, ValidationCode: fdriver.ValidationCode(r.code.Load()), Message: message}
	}
	return out, nil
}

func committed(txID string) *committerpb.NotificationResponse { return respFor(txID) }

func TestReconnect_PendingListenersSurvive(t *testing.T) {
	t.Parallel()
	nlm, _ := setupTest(t)
	fn := newFakeNotifier()
	s1, _ := startReconnecting(t, nlm, fn)

	l := &countingListener{}
	require.NoError(t, nlm.AddFinalityListener("tx1", l))
	s1.requested(t, "tx1")

	s1.fail()
	s2 := fn.next(t)
	require.Zero(t, l.calls.Load(), "a stream failure must not settle a pending listener")

	s2.requested(t, "tx1")
	s2.respond(t, committed("tx1"))
	require.Eventually(t, func() bool { return l.calls.Load() == 1 }, timeout, tick)
	require.Equal(t, int32(fdriver.Valid), l.status.Load())
}

func TestListenLoop_ReportsStateTransitions(t *testing.T) {
	t.Parallel()
	m, rec := connstatetest.NewMetrics()
	nlm, _ := setupTest(t)
	nlm.newConn = func(endpoint string) *connstate.Tracker {
		return m.Tracker("net", "ch", connstate.ComponentFabricxNotification, endpoint)
	}
	fn := newFakeNotifier()
	s1, stop := startReconnecting(t, nlm, fn)
	states := func() []connstate.State { return rec.States("committer") }
	require.Eventually(t, func() bool { return len(states()) == 2 }, timeout, tick)

	s1.fail()
	fn.next(t)
	require.Eventually(t, func() bool { return len(states()) == 4 }, timeout, tick)
	require.Equal(t, []connstate.State{connstate.Connecting, connstate.Connected, connstate.Reconnecting, connstate.Connected}, states())

	stop()
	last, _ := rec.Last("committer")
	require.Equal(t, connstate.Disconnected, last)
}

func TestReconnect_ReRequestsPending(t *testing.T) {
	t.Parallel()
	nlm, _ := setupTest(t)
	fn := newFakeNotifier()
	s1, _ := startReconnecting(t, nlm, fn)

	// More than one request's worth, so the re-request is split into batches.
	const n = maxTxIDsPerRequest + 5
	for i := range n {
		require.NoError(t, nlm.AddFinalityListener("tx"+strconv.Itoa(i), &countingListener{}))
	}
	s1.requestedAll(t, n)

	s1.fail()
	s2 := fn.next(t)

	for _, req := range s2.requestedAll(t, n) {
		require.LessOrEqual(t, len(req.GetTxStatusRequest().GetTxIds()), maxTxIDsPerRequest)
		require.Equal(t, testRequestTimeout, req.GetTimeout().AsDuration())
	}
}

func TestReconnect_AddDuringOutageIsRequestedOnReconnect(t *testing.T) {
	t.Parallel()
	nlm, _ := setupTest(t)
	fn := newFakeNotifier()
	s1, _ := startReconnecting(t, nlm, fn)

	fn.setOpenErr(errors.New("committer down"))
	s1.fail()
	require.Eventually(t, func() bool { return (*nlm.streamCtx.Load()).Err() != nil }, timeout, tick)

	l := &countingListener{}
	require.NoError(t, nlm.AddFinalityListener("tx1", l), "registration during an outage is kept, not rejected")

	fn.setOpenErr(nil)
	s2 := fn.next(t)
	s2.requested(t, "tx1")
	s2.respond(t, committed("tx1"))
	require.Eventually(t, func() bool { return l.calls.Load() == 1 }, timeout, tick)
}

func TestReconnect_AddBeforeFirstStreamDoesNotBlock(t *testing.T) {
	t.Parallel()
	nlm, _ := setupTest(t)
	fn := newFakeNotifier()
	fn.setOpenErr(errors.New("committer down at startup"))
	nlm.newNotifyClient = staticClient(fn)
	go func() { _ = nlm.run(t.Context()) }()

	done := make(chan error, 1)
	go func() { done <- nlm.AddFinalityListener("tx1", &countingListener{}) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(timeout):
		t.Fatal("AddFinalityListener blocked while no stream had ever opened")
	}

	fn.setOpenErr(nil)
	fn.next(t).requested(t, "tx1")
}

// Each reconnect asks newNotifyClient again, so a stream that breaks can reopen on another
// endpoint, and the txIDs still pending are requested there.
func TestReconnect_UsesNewClientAfterStreamFailure(t *testing.T) {
	t.Parallel()
	nlm, _ := setupTest(t)
	fnA, fnB := newFakeNotifier(), newFakeNotifier()
	var calls atomic.Int32
	nlm.newNotifyClient = func() (committerpb.SidecarServiceClient, string, error) {
		// simulates "A was healthy at first, then A died, so pick now returns B
		if calls.Add(1) == 1 {
			return fnA, "A", nil
		}
		return fnB, "B", nil
	}
	// Short enough that the reconnect never outgrows fakeNotifier.next's wait.
	// It starts the reconnect loop (run) in the background, as the node does
	nlm.reconnectMaxDelay = 50 * time.Millisecond
	go func() { _ = nlm.run(t.Context()) }()

	sA := fnA.next(t)
	require.Eventually(t, func() bool { return nlm.streamCtx.Load() != nil }, timeout, tick)
	require.NoError(t, nlm.AddFinalityListener("tx1", &countingListener{}))
	sA.requested(t, "tx1")

	sA.fail()
	fnB.next(t).requested(t, "tx1")
}

// The connection state follows the endpoint: after a reconnect lands on B, A's series ends
// disconnected instead of staying on its last state, and B gets a series of its own.
func TestReconnect_StateFollowsEndpoint(t *testing.T) {
	t.Parallel()
	m, rec := connstatetest.NewMetrics()
	nlm, _ := setupTest(t)
	nlm.newConn = func(endpoint string) *connstate.Tracker {
		return m.Tracker("net", "ch", connstate.ComponentFabricxNotification, endpoint)
	}
	fnA, fnB := newFakeNotifier(), newFakeNotifier()
	var calls atomic.Int32
	nlm.newNotifyClient = func() (committerpb.SidecarServiceClient, string, error) {
		if calls.Add(1) == 1 {
			return fnA, "A", nil
		}
		return fnB, "B", nil
	}
	nlm.reconnectMaxDelay = 50 * time.Millisecond
	go func() { _ = nlm.run(t.Context()) }()

	sA := fnA.next(t)
	require.Eventually(t, func() bool { return len(rec.States("A")) == 2 }, timeout, tick)
	sA.fail()
	fnB.next(t)
	require.Eventually(t, func() bool { return len(rec.States("B")) == 2 }, timeout, tick)

	require.Equal(t, []connstate.State{connstate.Connecting, connstate.Connected, connstate.Reconnecting, connstate.Disconnected}, rec.States("A"))
	require.Equal(t, []connstate.State{connstate.Reconnecting, connstate.Connected}, rec.States("B"))
}

// A failing newNotifyClient is one more failed attempt: run backs off and asks again
// rather than stopping the manager, and the stream opens once a client is available.
func TestReconnect_RetriesFailingClientFactory(t *testing.T) {
	t.Parallel()
	nlm, _ := setupTest(t)
	fn := newFakeNotifier()
	var calls atomic.Int32
	nlm.newNotifyClient = func() (committerpb.SidecarServiceClient, string, error) {
		if calls.Add(1) <= 2 {
			return nil, "", errors.New("no endpoint available")
		}
		return fn, "committer", nil
	}
	nlm.reconnectMaxDelay = 50 * time.Millisecond
	go func() { _ = nlm.run(t.Context()) }()

	require.NoError(t, nlm.AddFinalityListener("tx1", &countingListener{}))
	fn.next(t).requested(t, "tx1")
	require.Equal(t, int32(3), calls.Load(), "the stream opens on the first call that returns a client")
}

// A tx that commits while the stream is down is settled from the query service, and the
// listener is told exactly once: the committer's answers on the new stream, a timeout for
// the re-request and a late commit, reach nobody. The test runs in a synctest bubble, so
// each step happens after everything the previous one started has settled.
func TestReconnect_ResolvedTxNotifiedOnce(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		nlm, _ := setupTest(t)
		fn := newFakeNotifier()
		s1 := runInBubble(t, nlm, fn)

		resolver := &switchResolver{}
		resolver.code.Store(int32(fdriver.Unknown))
		rlm := NewResolvingListenerManager(nlm, resolver)
		l := &countingListener{}
		require.NoError(t, rlm.AddFinalityListener("tx1", l))
		<-s1.requests

		s1.fail()
		synctest.Wait()
		resolver.code.Store(int32(fdriver.Valid)) // committed during the gap
		s2 := <-fn.opened
		<-s2.requests
		synctest.Wait()
		require.Equal(t, int32(1), l.calls.Load())
		require.Equal(t, int32(fdriver.Valid), l.status.Load())

		s2.respond(t, &committerpb.NotificationResponse{TimeoutTxIds: []string{"tx1"}})
		s2.respond(t, committed("tx1"))
		synctest.Wait()
		require.Equal(t, int32(1), l.calls.Load())
	})
}

func TestReconnect_TTLStillExpires(t *testing.T) {
	t.Parallel()
	nlm, _ := setupTest(t)
	nlm.listenerTTL = 100 * time.Millisecond
	nlm.sweepInterval = 20 * time.Millisecond
	fn := newFakeNotifier()
	s1, _ := startReconnecting(t, nlm, fn)

	l := &countingListener{}
	require.NoError(t, nlm.AddFinalityListener("tx1", l))
	s1.requested(t, "tx1")

	fn.setOpenErr(errors.New("committer down"))
	s1.fail()
	require.Eventually(t, func() bool { return l.calls.Load() == 1 }, timeout, tick,
		"the TTL must settle the listener while the stream is down")
	time.Sleep(300 * time.Millisecond) // the outage lasts well past the TTL
	require.Equal(t, int32(1), l.calls.Load(), "the TTL settles exactly once")
	require.Equal(t, int32(fdriver.Unknown), l.status.Load())

	fn.setOpenErr(nil)
	s2 := fn.next(t)
	s2.respond(t, committed("tx1"))
	time.Sleep(shortWait)
	require.Equal(t, int32(1), l.calls.Load(), "an expired listener is not notified again")
}

func TestShutdown_SettlesUnknown(t *testing.T) {
	t.Parallel()
	nlm, _ := setupTest(t)
	fn := newFakeNotifier()
	s1, stop := startReconnecting(t, nlm, fn)

	l := &countingListener{}
	require.NoError(t, nlm.AddFinalityListener("tx1", l))
	s1.requested(t, "tx1")

	stop()
	require.Equal(t, int32(1), l.calls.Load())
	require.Equal(t, int32(fdriver.Unknown), l.status.Load())
	require.ErrorContains(t, nlm.AddFinalityListener("tx2", &countingListener{}), "notification stream unavailable",
		"a stopped manager rejects registrations")
}

func TestProvider_ManagerKeptAfterStreamFailure(t *testing.T) {
	t.Parallel()
	fn := newFakeNotifier()
	p := NewListenerManagerProvider(&mockGRPCClientProvider{}, nil, nil)
	p.newNotificationManager = func(string, GRPCClientProvider, config.Config) (*notificationListenerManager, error) {
		nlm, _ := setupTest(t)
		nlm.newNotifyClient = staticClient(fn)
		nlm.handlerTimeout = time.Second
		return nlm, nil
	}
	p.Initialize(t.Context())

	held, err := p.NewManager("net", "ch")
	require.NoError(t, err)
	fn.next(t).fail()
	s2 := fn.next(t)

	again, err := p.NewManager("net", "ch")
	require.NoError(t, err)
	require.Same(t, held, again, "the provider keeps the manager across stream failures")

	l := &countingListener{}
	require.NoError(t, held.AddFinalityListener("tx1", l))
	s2.requested(t, "tx1")
	s2.respond(t, committed("tx1"))
	require.Eventually(t, func() bool { return l.calls.Load() == 1 }, timeout, tick)
}

func TestRejectedSubscriptionSettlesUnknown(t *testing.T) {
	t.Parallel()
	nlm, _ := setupTest(t)
	fn := newFakeNotifier()
	s1, _ := startReconnecting(t, nlm, fn)

	l := &countingListener{}
	require.NoError(t, nlm.AddFinalityListener("tx1", l))
	s1.requested(t, "tx1")
	s1.respond(t, &committerpb.NotificationResponse{RejectedTxIds: &committerpb.RejectedTxIds{
		TxIds: []string{"tx1"}, Reason: "active tx IDs limit reached",
	}})

	require.Eventually(t, func() bool { return l.calls.Load() == 1 }, timeout, tick)
	require.Equal(t, int32(fdriver.Unknown), l.status.Load())
}

// runInBubble serves nlm over fn with run(), inside the caller's synctest bubble, and returns
// the first stream once it is attached. The manager is stopped when the test ends, so the
// bubble can finish.
func runInBubble(t *testing.T, nlm *notificationListenerManager, fn *fakeNotifier) *fakeStream {
	t.Helper()
	nlm.newNotifyClient = staticClient(fn)
	nlm.handlerTimeout = time.Second
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- nlm.run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	s := <-fn.opened
	// Wait until the stream is also attached, so that registrations from here on are sent
	// on it rather than re-requested, and resolved, with the stream's pending txIDs.
	synctest.Wait()
	return s
}

// A transaction that commits while the notification stream is down is not reported on the
// next stream: the committer reports only statuses that arrive after a subscription, and
// answers the re-request only once requestTimeout passes. The manager resolves the
// transaction from the query service instead, whether or not the stream is back. The test
// runs in a synctest bubble, so the clock advances only when every goroutine is blocked,
// and the listener must be settled well before requestTimeout.
func TestReconnect_ResolvesTxCommittedDuringOutage(t *testing.T) {
	t.Parallel()
	for _, reconnect := range []bool{true, false} {
		name := "stream stays down"
		if reconnect {
			name = "stream reconnects"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				nlm, _ := setupTest(t)
				fn := newFakeNotifier()
				s1 := runInBubble(t, nlm, fn)
				resolver := &switchResolver{}
				resolver.code.Store(int32(fdriver.Unknown))
				rlm := NewResolvingListenerManager(nlm, resolver)

				l := &countingListener{}
				require.NoError(t, rlm.AddFinalityListener("tx1", l))
				require.Equal(t, []string{"tx1"}, (<-s1.requests).GetTxStatusRequest().GetTxIds())

				if !reconnect {
					fn.setOpenErr(errors.New("committer down"))
				}
				s1.fail()
				// The failure is resolved at once, while the transaction is still pending;
				// it commits after that, during the outage, so only a later resolve finds it.
				synctest.Wait()
				require.Zero(t, l.calls.Load())
				committedMsg := "COMMITTED"
				resolver.message.Store(&committedMsg)
				resolver.code.Store(int32(fdriver.Valid))
				if reconnect {
					s2 := <-fn.opened
					// Re-requested; the committer stays silent, as it does until requestTimeout.
					require.Equal(t, []string{"tx1"}, (<-s2.requests).GetTxStatusRequest().GetTxIds())
				}

				time.Sleep(testRequestTimeout / 10)
				synctest.Wait()
				require.Equal(t, int32(1), l.calls.Load(), "the listener is settled from the query service")
				require.Equal(t, int32(fdriver.Valid), l.status.Load())
				require.Equal(t, committedMsg, *l.message.Load(), "with the query service's message")
			})
		})
	}
}

// While the stream is up, pending transactions are left to the committer's notifications:
// the manager does not query their status.
func TestReconnect_DoesNotResolveWhileStreamIsUp(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		nlm, _ := setupTest(t)
		fn := newFakeNotifier()
		s1 := runInBubble(t, nlm, fn)
		resolver := &switchResolver{}
		resolver.code.Store(int32(fdriver.Unknown))
		rlm := NewResolvingListenerManager(nlm, resolver)

		l := &countingListener{}
		require.NoError(t, rlm.AddFinalityListener("tx1", l))
		<-s1.requests
		require.Equal(t, int32(1), resolver.calls.Load(), "registration resolves once")

		time.Sleep(testRequestTimeout / 10)
		synctest.Wait()
		require.Equal(t, int32(1), resolver.calls.Load(), "no query while the stream is up")
		require.Zero(t, l.calls.Load())
	})
}
