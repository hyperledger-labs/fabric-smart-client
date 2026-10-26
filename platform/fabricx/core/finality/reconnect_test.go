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
	nlm.notifyClient = fn
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
	calls  atomic.Int32
	status atomic.Int32
}

func (l *countingListener) OnStatus(_ context.Context, _ driver.TxID, status int, _ string) {
	l.status.Store(int32(status))
	l.calls.Add(1)
}

// switchResolver answers every txID with code, which a test may change while
// listeners are pending.
type switchResolver struct{ code atomic.Int32 }

func (r *switchResolver) Statuses(_ context.Context, txIDs ...driver.TxID) ([]driver.TxValidationStatus[fdriver.ValidationCode], error) {
	out := make([]driver.TxValidationStatus[fdriver.ValidationCode], len(txIDs))
	for i, id := range txIDs {
		out[i] = driver.TxValidationStatus[fdriver.ValidationCode]{TxID: id, ValidationCode: fdriver.ValidationCode(r.code.Load())}
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
	nlm.conn = m.Tracker("net", "ch", connstate.ComponentFabricxNotification, "committer")
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
	nlm.notifyClient = fn
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

// The committer answers a tx that committed while the stream was down with a timeout,
// since it only reports statuses that arrive after the subscription. The resolving
// wrapper turns that Unknown into the real outcome from the query service, exactly once.
func TestReconnect_ResolvedTxNotifiedOnce(t *testing.T) {
	t.Parallel()
	nlm, _ := setupTest(t)
	fn := newFakeNotifier()
	s1, _ := startReconnecting(t, nlm, fn)

	resolver := &switchResolver{}
	resolver.code.Store(int32(fdriver.Unknown))
	rlm := NewResolvingListenerManager(nlm, resolver)
	l := &countingListener{}
	require.NoError(t, rlm.AddFinalityListener("tx1", l))
	s1.requested(t, "tx1")

	s1.fail()
	resolver.code.Store(int32(fdriver.Valid)) // committed during the gap
	s2 := fn.next(t)
	s2.requested(t, "tx1")
	s2.respond(t, &committerpb.NotificationResponse{TimeoutTxIds: []string{"tx1"}})

	require.Eventually(t, func() bool { return l.calls.Load() == 1 }, timeout, tick)
	require.Equal(t, int32(fdriver.Valid), l.status.Load())

	// A late duplicate on the stream reaches nobody: the entry is gone.
	s2.respond(t, committed("tx1"))
	time.Sleep(shortWait)
	require.Equal(t, int32(1), l.calls.Load())
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
		nlm.notifyClient = fn
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
