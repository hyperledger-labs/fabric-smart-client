/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// recordingListener captures the single status it is given.
type recordingListener struct {
	got chan driver.TxValidationStatus[fdriver.ValidationCode]
}

func newRecordingListener() *recordingListener {
	return &recordingListener{got: make(chan driver.TxValidationStatus[fdriver.ValidationCode], 4)}
}

func (l *recordingListener) OnStatus(_ context.Context, txID driver.TxID, vc fdriver.ValidationCode, msg string) {
	l.got <- driver.TxValidationStatus[fdriver.ValidationCode]{TxID: txID, ValidationCode: vc, Message: msg}
}

func (l *recordingListener) await(t *testing.T) driver.TxValidationStatus[fdriver.ValidationCode] {
	t.Helper()
	select {
	case st := <-l.got:
		return st
	case <-time.After(2 * time.Second):
		t.Fatal("listener was never invoked")
		return driver.TxValidationStatus[fdriver.ValidationCode]{}
	}
}

// fakeInner records registrations and lets a test drive the push path.
type fakeInner struct {
	added   chan fabric.FinalityListener
	removed chan fabric.FinalityListener
	addErr  error
}

func newFakeInner() *fakeInner {
	return &fakeInner{
		added:   make(chan fabric.FinalityListener, 4),
		removed: make(chan fabric.FinalityListener, 4),
	}
}

func (f *fakeInner) AddFinalityListener(_ driver.TxID, l fabric.FinalityListener) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.added <- l
	return nil
}

func (f *fakeInner) RemoveFinalityListener(_ driver.TxID, l fabric.FinalityListener) error {
	f.removed <- l
	return nil
}

// stubResolver answers with a fixed status, or an error.
type stubResolver struct {
	code fdriver.ValidationCode
	msg  string
	err  error
	// calls counts Statuses invocations.
	calls int
}

func (s *stubResolver) Statuses(_ context.Context, txIDs ...driver.TxID) ([]driver.TxValidationStatus[fdriver.ValidationCode], error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	out := make([]driver.TxValidationStatus[fdriver.ValidationCode], len(txIDs))
	for i, id := range txIDs {
		out[i] = driver.TxValidationStatus[fdriver.ValidationCode]{TxID: id, ValidationCode: s.code, Message: s.msg}
	}
	return out, nil
}

func TestResolvingSettlesAlreadyFinalWithoutSubscribing(t *testing.T) {
	t.Parallel()

	for _, code := range []fdriver.ValidationCode{fdriver.Valid, fdriver.Invalid} {
		inner := newFakeInner()
		res := &stubResolver{code: code, msg: "COMMITTED"}
		m := NewResolvingListenerManager(inner, res)
		l := newRecordingListener()

		require.NoError(t, m.AddFinalityListener("tx1", l))

		st := l.await(t)
		assert.Equal(t, code, st.ValidationCode)
		assert.Equal(t, "COMMITTED", st.Message)
		assert.Empty(t, inner.added, "must not subscribe for an already-final transaction")
	}
}

func TestResolvingSubscribesWhenNotFinal(t *testing.T) {
	t.Parallel()

	inner := newFakeInner()
	m := NewResolvingListenerManager(inner, &stubResolver{code: fdriver.Unknown})
	l := newRecordingListener()

	require.NoError(t, m.AddFinalityListener("tx1", l))
	require.Len(t, inner.added, 1)
	assert.Empty(t, l.got, "listener must not be settled before a status arrives")
}

func TestResolvingForwardsPushedStatus(t *testing.T) {
	t.Parallel()

	inner := newFakeInner()
	m := NewResolvingListenerManager(inner, &stubResolver{code: fdriver.Unknown})
	l := newRecordingListener()
	require.NoError(t, m.AddFinalityListener("tx1", l))

	proxy := <-inner.added
	proxy.OnStatus(context.Background(), "tx1", fdriver.Valid, "COMMITTED")

	st := l.await(t)
	assert.Equal(t, fdriver.Valid, st.ValidationCode)
}

func TestResolvingReResolvesOnUnknown(t *testing.T) {
	t.Parallel()

	inner := newFakeInner()
	res := &stubResolver{code: fdriver.Unknown}
	m := NewResolvingListenerManager(inner, res)
	l := newRecordingListener()
	require.NoError(t, m.AddFinalityListener("tx1", l))
	proxy := <-inner.added

	// By the time the nlm gives up, the committer knows the answer.
	res.code, res.msg = fdriver.Valid, "COMMITTED"
	proxy.OnStatus(context.Background(), "tx1", fdriver.Unknown, "committer reported a timeout")

	st := l.await(t)
	assert.Equal(t, fdriver.Valid, st.ValidationCode, "Unknown must be re-resolved, not passed on")
}

func TestResolvingPassesUnknownThrough(t *testing.T) {
	t.Parallel()

	// Both cases mean "still nobody knows", so Unknown reaches the caller either way.
	for name, reResolveErr := range map[string]error{
		"still not final":  nil,
		"resolver errored": errors.New("query service unreachable"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			inner := newFakeInner()
			res := &stubResolver{code: fdriver.Unknown}
			m := NewResolvingListenerManager(inner, res)
			l := newRecordingListener()
			require.NoError(t, m.AddFinalityListener("tx1", l))
			proxy := <-inner.added

			res.err = reResolveErr
			proxy.OnStatus(context.Background(), "tx1", fdriver.Unknown, "committer reported a timeout")

			st := l.await(t)
			assert.Equal(t, fdriver.Unknown, st.ValidationCode)
			assert.Equal(t, 2, res.calls, "exactly one re-resolve, with no re-subscribe loop")
			assert.Empty(t, inner.added)
		})
	}
}

func TestResolvingSubscribesWhenResolverErrorsAtRegistration(t *testing.T) {
	t.Parallel()

	inner := newFakeInner()
	m := NewResolvingListenerManager(inner, &stubResolver{err: errors.New("query service unreachable")})
	l := newRecordingListener()

	require.NoError(t, m.AddFinalityListener("tx1", l), "a resolver failure must not fail the registration")
	assert.Len(t, inner.added, 1, "must degrade to the notification path")
}

func TestResolvingRejectsEmptyTxID(t *testing.T) {
	t.Parallel()

	inner := newFakeInner()
	m := NewResolvingListenerManager(inner, &stubResolver{code: fdriver.Unknown})

	require.Error(t, m.AddFinalityListener("", newRecordingListener()))
	assert.Empty(t, inner.added)
}

func TestResolvingPropagatesInnerAddError(t *testing.T) {
	t.Parallel()

	inner := newFakeInner()
	inner.addErr = errors.New("stream unavailable")
	m := NewResolvingListenerManager(inner, &stubResolver{code: fdriver.Unknown})

	require.Error(t, m.AddFinalityListener("tx1", newRecordingListener()))
}

func TestResolvingRemoveAfterImmediateSettleIsNoOp(t *testing.T) {
	t.Parallel()

	inner := newFakeInner()
	m := NewResolvingListenerManager(inner, &stubResolver{code: fdriver.Valid})
	l := newRecordingListener()
	require.NoError(t, m.AddFinalityListener("tx1", l))
	l.await(t)

	require.NoError(t, m.RemoveFinalityListener("tx1", l))
	assert.Empty(t, inner.removed, "nothing was subscribed, so nothing to remove")
}

func TestResolvingRemoveUnsubscribesTheProxy(t *testing.T) {
	t.Parallel()

	inner := newFakeInner()
	m := NewResolvingListenerManager(inner, &stubResolver{code: fdriver.Unknown})
	l := newRecordingListener()
	require.NoError(t, m.AddFinalityListener("tx1", l))
	proxy := <-inner.added

	require.NoError(t, m.RemoveFinalityListener("tx1", l))
	require.Len(t, inner.removed, 1)
	assert.Same(t, proxy, <-inner.removed, "the proxy must be unsubscribed, not the caller's listener")
}

func TestResolvingRemoveIsScopedToOneTransaction(t *testing.T) {
	t.Parallel()

	// One listener object registered for two transactions: each subscription has its own
	// proxy, and removing one must leave the other in place.
	inner := newFakeInner()
	m := NewResolvingListenerManager(inner, &stubResolver{code: fdriver.Unknown})
	l := newRecordingListener()

	require.NoError(t, m.AddFinalityListener("tx1", l))
	proxy1 := <-inner.added
	require.NoError(t, m.AddFinalityListener("tx2", l))
	proxy2 := <-inner.added
	require.NotSame(t, proxy1, proxy2, "each transaction needs its own proxy")

	require.NoError(t, m.RemoveFinalityListener("tx1", l))
	require.Len(t, inner.removed, 1)
	assert.Same(t, proxy1, <-inner.removed)

	require.NoError(t, m.RemoveFinalityListener("tx2", l))
	require.Len(t, inner.removed, 1, "tx2 must still have been subscribed after tx1 was removed")
	assert.Same(t, proxy2, <-inner.removed, "the proxy removed must be the one registered for tx2")
}

func TestResolvingForgetsSubscriptionOnceDelivered(t *testing.T) {
	t.Parallel()

	// A caller that registers, is called back once and moves on must leave nothing behind:
	// the wrapped manager auto-removes a listener as it settles it, so no further
	// RemoveFinalityListener is coming for this subscription.
	inner := newFakeInner()
	m := NewResolvingListenerManager(inner, &stubResolver{code: fdriver.Unknown})
	l := newRecordingListener()
	require.NoError(t, m.AddFinalityListener("tx1", l))
	require.Len(t, m.proxies, 1)

	proxy := <-inner.added
	proxy.OnStatus(context.Background(), "tx1", fdriver.Valid, "COMMITTED")

	assert.Empty(t, m.proxies, "a delivered subscription must not stay tracked")
	assert.Equal(t, fdriver.Valid, l.await(t).ValidationCode)
}

func TestResolvingDeduplicatesTheSameListener(t *testing.T) {
	t.Parallel()

	// The wrapped manager refuses the same listener instance twice for one txID. A fresh
	// proxy per call hides that: the caller would be invoked twice, and only one of the
	// two subscriptions would be reachable through RemoveFinalityListener.
	inner := newFakeInner()
	m := NewResolvingListenerManager(inner, &stubResolver{code: fdriver.Unknown})
	l := newRecordingListener()

	require.NoError(t, m.AddFinalityListener("tx1", l))
	require.NoError(t, m.AddFinalityListener("tx1", l), "a duplicate registration reports success")
	require.Len(t, inner.added, 1, "only one subscription may reach the wrapped manager")

	proxy := <-inner.added
	proxy.OnStatus(context.Background(), "tx1", fdriver.Valid, "COMMITTED")
	assert.Len(t, l.got, 1, "the caller's listener must be invoked exactly once")
}

func TestResolvingReResolvesOnTheCallingGoroutine(t *testing.T) {
	t.Parallel()

	// Bounding the re-resolve matters: the wrapped manager settles every pending listener
	// with Unknown when its stream tears down, so one goroutine per listener would fan out
	// a query per pending transaction at the moment the committer became unreachable.
	// Staying on the caller keeps them bounded by its handler pool.
	inner := newFakeInner()
	res := &stubResolver{code: fdriver.Unknown}
	m := NewResolvingListenerManager(inner, res)
	l := newRecordingListener()
	require.NoError(t, m.AddFinalityListener("tx1", l))
	proxy := <-inner.added

	res.code = fdriver.Valid
	proxy.OnStatus(context.Background(), "tx1", fdriver.Unknown, "committer reported a timeout")

	// Deliberately not l.await: OnStatus must not return before the caller was invoked.
	require.Len(t, l.got, 1, "the re-resolve must complete before OnStatus returns")
	assert.Equal(t, fdriver.Valid, (<-l.got).ValidationCode)
}

func TestResolvingKeepsALiveProxyWhenAnOlderOneDelivers(t *testing.T) {
	t.Parallel()

	// A callback the wrapped manager already handed off can run after the caller removed
	// and re-registered the same listener. The older proxy must forget only its own
	// subscription: forgetting by key alone would leave the live one subscribed with
	// nothing tracking it, and RemoveFinalityListener would then silently do nothing.
	inner := newFakeInner()
	m := NewResolvingListenerManager(inner, &stubResolver{code: fdriver.Unknown})
	l := newRecordingListener()

	require.NoError(t, m.AddFinalityListener("tx1", l))
	stale := <-inner.added
	require.NoError(t, m.RemoveFinalityListener("tx1", l))
	<-inner.removed

	require.NoError(t, m.AddFinalityListener("tx1", l))
	live := <-inner.added
	require.NotSame(t, stale, live)

	stale.OnStatus(context.Background(), "tx1", fdriver.Valid, "COMMITTED")

	require.NoError(t, m.RemoveFinalityListener("tx1", l))
	require.Len(t, inner.removed, 1)
	assert.Same(t, live, <-inner.removed, "the live subscription must still be reachable")
}
