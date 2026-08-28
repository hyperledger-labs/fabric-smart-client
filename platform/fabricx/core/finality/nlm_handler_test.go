/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality/mock"
)

// countingBlockingListener blocks forever on every call and records how many
// calls are in flight, so a test can assert the worker count is respected.
type countingBlockingListener struct {
	block    chan struct{}
	inFlight atomic.Int32
	peak     atomic.Int32
	// admitted counts every OnStatus entry over the listener's lifetime, i.e.
	// how much work the manager actually let through.
	admitted atomic.Int32
}

func newCountingBlockingListener() *countingBlockingListener {
	return &countingBlockingListener{block: make(chan struct{})}
}

func (c *countingBlockingListener) OnStatus(_ context.Context, _ string, _ int, _ string) {
	c.admitted.Add(1)
	n := c.inFlight.Add(1)
	for {
		peak := c.peak.Load()
		if n <= peak || c.peak.CompareAndSwap(peak, n) {
			break
		}
	}
	<-c.block
	c.inFlight.Add(-1)
}

// funcListener adapts a function to the FinalityListener interface.
type funcListener struct {
	fn func(ctx context.Context, txID string, status int, statusMessage string)
}

func (f *funcListener) OnStatus(ctx context.Context, txID string, status int, statusMessage string) {
	f.fn(ctx, txID, status, statusMessage)
}

// respFor builds a notification response marking every txID as committed.
func respFor(txIDs ...string) *committerpb.NotificationResponse {
	events := make([]*committerpb.TxStatus, 0, len(txIDs))
	for _, txID := range txIDs {
		events = append(events, &committerpb.TxStatus{
			Ref:    &committerpb.TxRef{TxId: txID},
			Status: committerpb.Status_COMMITTED,
		})
	}
	return &committerpb.NotificationResponse{TxStatusEvents: events}
}

// feedResponses makes Recv return each response once, in order, then park until
// the context is done.
func feedResponses(ctx context.Context, fakeStream *mock.Notifier_OpenNotificationStreamClient, responses ...*committerpb.NotificationResponse) {
	var idx atomic.Int32
	fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
		i := int(idx.Add(1)) - 1
		if i < len(responses) {
			return responses[i], nil
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
}

const (
	// listenReturnTimeout bounds how long a test waits for listen() to return.
	listenReturnTimeout = 2 * time.Second

	// testHandlerTimeout stands in for config.DefaultHandlerTimeout. These tests only
	// need it non-zero, and teardown waits it out when a listener ignores its context
	// -- 5s per test for nothing. Short, so a slow listener costs 200ms, not 5s.
	testHandlerTimeout = 200 * time.Millisecond
)

// requireListenReturned waits for listen() to return, also honouring the test's own
// context so a suite-level cancellation is not ignored.
func requireListenReturned(t *testing.T, listenErr <-chan error) {
	t.Helper()
	select {
	case <-listenErr:
	case <-t.Context().Done():
		t.Fatal("test context cancelled before listen() returned")
	case <-time.After(listenReturnTimeout):
		t.Fatal("listen() did not return: teardown is blocked")
	}
}

func TestHandlerPool(t *testing.T) {
	t.Parallel()

	t.Run("Worker_Count_Caps_Concurrent_Callbacks", func(t *testing.T) {
		// The regression test for the unbounded-goroutine bug. Before the limit,
		// every (txID, listener) pair got its own pair of goroutines, so concurrent
		// OnStatus calls tracked the notification count. Now the number of worker
		// goroutines is the ceiling, whatever the rate.
		t.Parallel()

		const limit = 4
		const notifications = 40

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout
		nlm.handlerWorkers = limit

		listener := newCountingBlockingListener()
		t.Cleanup(func() { close(listener.block) })

		txIDs := make([]string, 0, notifications)
		for i := range notifications {
			txID := "tx_capped_" + strconv.Itoa(i)
			txIDs = append(txIDs, txID)
			seedHandlers(nlm, txID, listener)
		}

		feedResponses(t.Context(), fakeStream, respFor(txIDs...))
		runManager(t, nlm)

		// Wait until the limit is saturated.
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Equal(collect, int32(limit), listener.inFlight.Load())
		}, timeout, tick, "all handler slots should be held by the blocked listener")

		// Give the dispatcher room to misbehave, then assert it did not.
		time.Sleep(shortWait)
		require.LessOrEqual(t, listener.peak.Load(), int32(limit),
			"concurrent OnStatus calls must never exceed the worker count")
	})

	t.Run("Blocked_Listeners_Admit_At_Most_The_Limit", func(t *testing.T) {
		// With every worker held by a listener that never returns, nothing retires, so
		// admitted work stays at the worker count however many notifications arrive.
		//
		// Avoids runtime.NumGoroutine() deliberately: it is process-global, so
		// parallel sibling tests running their own managers make it meaningless.
		t.Parallel()

		const limit = 2

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout
		nlm.handlerWorkers = limit

		listener := newCountingBlockingListener()
		t.Cleanup(func() { close(listener.block) })

		const batches = 10
		const perBatch = 20
		responses := make([]*committerpb.NotificationResponse, 0, batches)
		for b := range batches {
			txIDs := make([]string, 0, perBatch)
			for i := range perBatch {
				txID := "tx_flat_" + strconv.Itoa(b) + "_" + strconv.Itoa(i)
				txIDs = append(txIDs, txID)
				seedHandlers(nlm, txID, listener)
			}
			responses = append(responses, respFor(txIDs...))
		}

		feedResponses(t.Context(), fakeStream, responses...)
		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Equal(collect, int32(limit), listener.inFlight.Load())
		}, timeout, tick, "handler slots should be held")

		// Give the dispatcher ample time to push all 200 notifications through.
		time.Sleep(shortWait)

		require.LessOrEqual(t, listener.peak.Load(), int32(limit),
			"concurrent OnStatus calls must never exceed the worker count")
		require.LessOrEqual(t, listener.admitted.Load(), int32(limit),
			"with every worker held forever, admitted work must stay at the worker count (%d), not track the %d notifications delivered",
			limit, batches*perBatch)
	})

	t.Run("Saturation_Does_Not_Block_Dispatcher", func(t *testing.T) {
		// A saturated pool must not stall the dispatcher. Asserted by it draining the
		// whole batch, not by an empty handlers map: listeners it cannot hand off are
		// deliberately kept for the sweeper, so entries legitimately remain.
		t.Parallel()

		nlm, fakeStream := setupTest(t)
		// Short timeout and several workers so teardown does not serialise 40+ blocked
		// listeners at handlerTimeout each: kept listeners all reach settleAllAndClear.
		nlm.handlerTimeout = 20 * time.Millisecond
		nlm.handlerWorkers = 8
		nlm.handlerQueueSize = 2 // tiny, so the flood saturates it

		blocker := newCountingBlockingListener()
		t.Cleanup(func() { close(blocker.block) })

		const floodSize = 50
		floodTxIDs := make([]string, 0, floodSize)
		for i := range floodSize {
			txID := "tx_flood_" + strconv.Itoa(i)
			floodTxIDs = append(floodTxIDs, txID)
			seedHandlers(nlm, txID, blocker)
		}

		feedResponses(t.Context(), fakeStream, respFor(floodTxIDs...))
		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Positive(collect, blocker.inFlight.Load(), "handler slots should be held")
		}, timeout, tick)

		// The dispatcher worked through the entire response rather than blocking on the
		// full queue: every txID was either handed to the pool or kept for the sweeper,
		// and none was left untouched.
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			nlm.handlersMu.RLock()
			remaining := len(nlm.handlers)
			nlm.handlersMu.RUnlock()
			assert.Less(collect, remaining, floodSize,
				"dispatcher must process the batch even when the queue is full")
		}, timeout, tick)
	})

	t.Run("Slots_Are_Reused_Across_Batches", func(t *testing.T) {
		// Workers are a concurrency ceiling, not a lifetime quota: without release,
		// the manager would deliver exactly handlerWorkers callbacks and then go
		// silent forever. One txID per batch, so the queue is never contended.
		t.Parallel()

		const limit = 2
		const batches = 24

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout
		nlm.handlerWorkers = limit

		var calls atomic.Int32
		listener := &funcListener{fn: func(context.Context, string, int, string) {
			calls.Add(1)
		}}

		responses := make([]*committerpb.NotificationResponse, 0, batches)
		for i := range batches {
			txID := "tx_reuse_" + strconv.Itoa(i)
			seedHandlers(nlm, txID, listener)
			responses = append(responses, respFor(txID))
		}

		feedResponses(t.Context(), fakeStream, responses...)
		runManager(t, nlm)

		// All of them, not merely more than the worker count: workers return to the
		// queue as callbacks finish.
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Equal(collect, int32(batches), calls.Load())
		}, timeout, tick,
			"every batch must be delivered; workers are not returning to the queue")
	})

	t.Run("Burst_Larger_Than_Limit_Is_Delivered_In_Full", func(t *testing.T) {
		// Why the queue exists: one response can carry far more transactions than there
		// are workers, so without a buffer the surplus would fall back to the sweeper
		// even with healthy listeners. Concurrency must still be capped -- see peak.
		t.Parallel()

		const limit = 4
		const burst = 200

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout
		nlm.handlerWorkers = limit

		var ran atomic.Int32
		var inFlight, peak atomic.Int32
		// Returns immediately: nothing here is misbehaving.
		listener := &funcListener{fn: func(context.Context, string, int, string) {
			cur := inFlight.Add(1)
			for {
				p := peak.Load()
				if cur <= p || peak.CompareAndSwap(p, cur) {
					break
				}
			}
			ran.Add(1)
			inFlight.Add(-1)
		}}

		txIDs := make([]string, 0, burst)
		for i := range burst {
			txID := "tx_burst_" + strconv.Itoa(i)
			txIDs = append(txIDs, txID)
			seedHandlers(nlm, txID, listener)
		}

		feedResponses(t.Context(), fakeStream, respFor(txIDs...))
		runManager(t, nlm)

		// Every callback in the burst must run -- none dropped.
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Equal(collect, int32(burst), ran.Load())
		}, timeout, tick,
			"a burst of %d against a limit of %d must be delivered in full: the queue exists to absorb it",
			burst, limit)

		// ...and the limit must still have held throughout.
		require.LessOrEqual(t, peak.Load(), int32(limit),
			"delivering the burst must not exceed the concurrency limit")
	})

	t.Run("Slow_Listener_Does_Not_Stall_Other_TxIDs", func(t *testing.T) {
		// One slow-but-completing listener must not delay other txIDs, as long as
		// a slot is free.
		t.Parallel()

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout
		nlm.handlerWorkers = 4

		// Slow relative to fast, but inside testHandlerTimeout so it still completes.
		slow := &delayedListener{delay: 80 * time.Millisecond}
		slow.expect(1)
		fast := &mockListener{}
		fast.expect(1)

		seedHandlers(nlm, "tx_slow", slow)
		seedHandlers(nlm, "tx_fast", fast)

		feedResponses(t.Context(), fakeStream, respFor("tx_slow", "tx_fast"))
		runManager(t, nlm)

		// fast must land well before slow's delay elapses.
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			txID, status := fast.getStatus()
			assert.Equal(collect, "tx_fast", txID)
			assert.Equal(collect, fdriver.Valid, status)
		}, 60*time.Millisecond, tick, "fast listener must not wait behind the slow one")

		// slow still completes.
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			txID, status := slow.getStatus()
			assert.Equal(collect, "tx_slow", txID)
			assert.Equal(collect, fdriver.Valid, status)
		}, timeout, tick, "slow listener should still complete")
	})

	t.Run("Teardown_Not_Blocked_By_Stuck_Callback", func(t *testing.T) {
		// This is why the workers are tracked separately from the stream goroutines:
		// a callback that ignores cancellation keeps its goroutine alive, so waiting
		// on it would never return. listen() must still return.
		t.Parallel()

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = 100 * time.Millisecond
		nlm.handlerWorkers = 4

		// Derived from t.Context() so the test's own deadline or failure also
		// tears this down; cancel() is what drives the teardown under test.
		ctx, cancel := context.WithCancel(t.Context())
		stuck := newCountingBlockingListener()
		t.Cleanup(func() { close(stuck.block) })
		seedHandlers(nlm, "tx_stuck_in_flight", stuck)

		feedResponses(ctx, fakeStream, respFor("tx_stuck_in_flight"))

		listenErr := make(chan error, 1)
		go func() { listenErr <- nlm.listen(ctx) }()

		// Wait until the callback is actually inside OnStatus.
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Equal(collect, int32(1), stuck.inFlight.Load())
		}, timeout, tick, "the stuck callback should be in flight")

		cancel()
		requireListenReturned(t, listenErr)
	})

	t.Run("Callbacks_Never_Get_A_Cancelled_Context", func(t *testing.T) {
		// listen() is usually returning *because* its context was cancelled, so a callback
		// that inherited it would be handed an already-expired timeout and report an
		// outcome nobody ever waited for. The backlog is what makes this reachable:
		// callbacks queued before the cancel run after it.
		t.Parallel()

		const batch = 60

		nlm, fakeStream := setupTest(t)
		// Generous, so the drain is never the thing under test here.
		nlm.handlerTimeout = time.Second
		// One worker against a queue big enough for the whole batch, so most of it is
		// still buffered when the cancel lands.
		nlm.handlerWorkers = 1
		nlm.handlerQueueSize = batch

		var live, dead atomic.Int32
		listener := &funcListener{fn: func(ctx context.Context, _ string, _ int, _ string) {
			if ctx.Err() != nil {
				dead.Add(1)
			} else {
				live.Add(1)
			}
			time.Sleep(time.Millisecond)
		}}

		ids := make([]string, 0, batch)
		for i := range batch {
			id := "tx_cancelctx_" + strconv.Itoa(i)
			ids = append(ids, id)
			seedHandlers(nlm, id, listener)
		}

		ctx, cancel := context.WithCancel(t.Context())
		feedResponses(ctx, fakeStream, respFor(ids...))

		listenErr := make(chan error, 1)
		go func() { listenErr <- nlm.listen(ctx) }()

		// Cancel once the pool is running, with the rest of the batch still queued.
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Positive(collect, live.Load()+dead.Load(), "the pool should have started")
		}, timeout, tick)
		cancel()

		requireListenReturned(t, listenErr)

		require.Equal(t, int32(batch), live.Load()+dead.Load(),
			"every queued callback must run: close(q) ends the workers' range only once the buffer is empty")
		require.Zero(t, dead.Load(),
			"%d of %d callbacks were handed an already-cancelled context; they must never see one",
			dead.Load(), batch)
	})

	t.Run("Unqueueable_Callbacks_Stay_In_The_Map", func(t *testing.T) {
		// dispatch must not delete a listener it could not hand to the pool: the
		// sweeper only sees n.handlers, so an orphaned listener never gets OnStatus at
		// all -- not even Unknown.
		t.Parallel()

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout
		nlm.handlerWorkers = 1
		nlm.handlerQueueSize = 1 // fills immediately

		blocker := newCountingBlockingListener()
		t.Cleanup(func() { close(blocker.block) })

		const batch = 20
		ids := make([]string, 0, batch)
		for i := range batch {
			id := "tx_kept_" + strconv.Itoa(i)
			ids = append(ids, id)
			seedHandlers(nlm, id, blocker)
		}

		feedResponses(t.Context(), fakeStream, respFor(ids...))
		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Positive(collect, blocker.inFlight.Load(), "the worker should be held")
		}, timeout, tick)

		// Whatever could not be queued must still be tracked, so the sweeper owns it.
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			nlm.handlersMu.RLock()
			remaining := len(nlm.handlers)
			nlm.handlersMu.RUnlock()
			assert.Positive(collect, remaining,
				"un-queued listeners were deleted anyway: nothing can settle them now")
		}, timeout, tick)
	})

	t.Run("Unqueueable_Callbacks_Are_Eventually_Settled", func(t *testing.T) {
		// The point of keeping them: every registered listener still gets exactly one
		// OnStatus. Counts any status, not Unknown specifically -- a kept listener whose
		// answer did arrive is settled with the real one; see
		// Kept_Callbacks_Are_Retried_With_The_Real_Status.
		t.Parallel()

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout
		nlm.handlerWorkers = 1
		nlm.handlerQueueSize = 1
		nlm.listenerTTL = 40 * time.Millisecond
		nlm.sweepInterval = 20 * time.Millisecond

		var settled atomic.Int32
		counter := &funcListener{fn: func(context.Context, string, int, string) {
			settled.Add(1)
		}}

		const batch = 10
		ids := make([]string, 0, batch)
		for i := range batch {
			id := "tx_settled_" + strconv.Itoa(i)
			ids = append(ids, id)
			seedHandlers(nlm, id, counter)
			setExpiry(nlm, id, time.Now().Add(nlm.listenerTTL))
		}

		feedResponses(t.Context(), fakeStream, respFor(ids...))
		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Positive(collect, settled.Load(),
				"listeners kept in the map must eventually be settled by the sweeper")
		}, 3*time.Second, tick)
	})

	t.Run("Kept_Callbacks_Are_Retried_With_The_Real_Status", func(t *testing.T) {
		// A committer answer that arrives but cannot be queued must not be downgraded
		// to Unknown. The status is remembered on the entry and used when the sweep
		// retries it, so a transaction that committed is reported Valid even though
		// its callback was delayed.
		t.Parallel()

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout
		nlm.handlerWorkers = 2
		nlm.handlerQueueSize = 2 // tiny, so most of the batch is kept
		nlm.listenerTTL = 40 * time.Millisecond
		nlm.sweepInterval = 20 * time.Millisecond

		var valid, unknown, other atomic.Int32
		listener := &funcListener{fn: func(_ context.Context, _ string, status int, _ string) {
			switch status {
			case fdriver.Valid:
				valid.Add(1)
			case fdriver.Unknown:
				unknown.Add(1)
			default:
				other.Add(1)
			}
		}}

		const batch = 24
		ids := make([]string, 0, batch)
		for i := range batch {
			id := "tx_realstatus_" + strconv.Itoa(i)
			ids = append(ids, id)
			seedHandlers(nlm, id, listener)
		}

		// respFor marks every txID COMMITTED, i.e. fdriver.Valid.
		feedResponses(t.Context(), fakeStream, respFor(ids...))
		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Equal(collect, int32(batch), valid.Load()+unknown.Load()+other.Load(),
				"every listener should have been settled")
		}, 3*time.Second, tick)

		require.Zero(t, unknown.Load(),
			"%d listeners were settled Unknown despite the committer reporting COMMITTED",
			unknown.Load())
		require.Equal(t, int32(batch), valid.Load(), "all should carry the real status")
	})

	t.Run("Kept_Without_A_Response_Is_Settled_Unknown", func(t *testing.T) {
		// The other side: a listener that expires without the committer ever answering
		// has no remembered status, so Unknown is correct.
		t.Parallel()

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout
		nlm.handlerWorkers = 2
		nlm.listenerTTL = 30 * time.Millisecond
		nlm.sweepInterval = 15 * time.Millisecond

		var unknown atomic.Int32
		listener := &funcListener{fn: func(_ context.Context, _ string, status int, _ string) {
			if status == fdriver.Unknown {
				unknown.Add(1)
			}
		}}

		seedHandlers(nlm, "tx_silent", listener)
		setExpiry(nlm, "tx_silent", time.Now().Add(nlm.listenerTTL))

		// Recv never returns a response: nothing is ever heard for this txID.
		ctx := t.Context()
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Equal(collect, int32(1), unknown.Load())
		}, 3*time.Second, tick,
			"a listener with no committer answer must be settled Unknown")
	})

	t.Run("Kept_Callbacks_Are_Retried_Even_Without_Local_Expiry", func(t *testing.T) {
		// A kept listener must be reachable by the sweeper whatever its original
		// deadline was. sweepExpired skips entries whose expiresAt is zero ("never
		// expires"), which is what AddFinalityListener stamps when listenerTTL is 0 --
		// so without a fresh deadline a kept listener would wait for stream teardown.
		t.Parallel()

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout
		nlm.handlerWorkers = 2
		nlm.handlerQueueSize = 2 // tiny, so the batch overflows
		// Sweeper on, but the entries themselves carry no deadline: seedHandlers
		// leaves expiresAt zero, exactly as listenerTTL == 0 would.
		nlm.listenerTTL = 40 * time.Millisecond
		nlm.sweepInterval = 20 * time.Millisecond

		var settled atomic.Int32
		quick := &funcListener{fn: func(context.Context, string, int, string) {
			settled.Add(1)
		}}

		const batch = 30
		ids := make([]string, 0, batch)
		for i := range batch {
			id := "tx_noexpiry_" + strconv.Itoa(i)
			ids = append(ids, id)
			seedHandlers(nlm, id, quick) // expiresAt stays zero
		}

		feedResponses(t.Context(), fakeStream, respFor(ids...))
		runManager(t, nlm)

		// Workers are free and the queue drains, so every listener must get through --
		// the ones kept on the first pass included.
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Equal(collect, int32(batch), settled.Load())
		}, 3*time.Second, tick,
			"kept listeners were never retried: they carry no deadline the sweeper will act on")
	})

	t.Run("Teardown_Is_Bounded_Regardless_Of_Pending_Listeners", func(t *testing.T) {
		// Teardown costs one handlerTimeout in total, not one per listener: it settles
		// everything into the queue, closes it, and waits once. A listener that ignores
		// its context keeps its worker -- nothing can force it to return -- so the wait is
		// abandoned rather than paid per stuck callback.
		t.Parallel()

		const pending = 200
		const hTimeout = 100 * time.Millisecond

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = hTimeout
		nlm.handlerWorkers = 1

		block := make(chan struct{}) // never closed: every listener hangs
		t.Cleanup(func() { close(block) })
		for i := range pending {
			seedHandlers(nlm, "tx_settle_"+strconv.Itoa(i), &funcListener{
				fn: func(context.Context, string, int, string) { <-block },
			})
		}

		ctx, cancel := context.WithCancel(t.Context())
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		listenErr := make(chan error, 1)
		go func() { listenErr <- nlm.listen(ctx) }()

		cancel()
		start := time.Now()
		requireListenReturned(t, listenErr)
		elapsed := time.Since(start)

		require.Less(t, elapsed, 4*hTimeout,
			"tearing down with %d stuck listeners took %s: teardown scales with the backlog instead of being bounded by handlerTimeout (%s)",
			pending, elapsed, hTimeout)
	})

	t.Run("Teardown_Drains_Queued_Callbacks", func(t *testing.T) {
		// Callbacks already queued when the stream stops must still be delivered. dispatch
		// has removed their handlers entries by then, so settleAllAndClear will not settle
		// them: dropping them here loses the notification outright, with no Unknown
		// fallback. close(q) is what guarantees it -- the workers' range ends only once the
		// buffer is empty.
		t.Parallel()

		nlm, fakeStream := setupTest(t)
		// Generous, so a slow drain is not mistaken for a lost callback.
		nlm.handlerTimeout = time.Second
		// One slow worker, so the batch backs up in the queue rather than draining as
		// fast as it is dispatched.
		nlm.handlerWorkers = 1
		nlm.handlerQueueSize = 200

		var ran atomic.Int32
		slow := &funcListener{fn: func(context.Context, string, int, string) {
			ran.Add(1)
			time.Sleep(5 * time.Millisecond)
		}}

		const batch = 20
		ids := make([]string, 0, batch)
		for i := range batch {
			id := "tx_drain_" + strconv.Itoa(i)
			ids = append(ids, id)
			seedHandlers(nlm, id, slow)
		}

		ctx, cancel := context.WithCancel(t.Context())
		feedResponses(ctx, fakeStream, respFor(ids...))

		listenErr := make(chan error, 1)
		go func() { listenErr <- nlm.listen(ctx) }()

		// Cancel with a backlog still buffered: at 5ms each the worker cannot have got
		// through the batch yet.
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Positive(collect, ran.Load(), "the batch should have reached the pool")
		}, timeout, tick)
		cancel()

		requireListenReturned(t, listenErr)

		require.Equal(t, int32(batch), ran.Load(),
			"every queued callback must be delivered on teardown, not discarded with the workers")
	})

	t.Run("Teardown_Settles_Listeners_Left_In_The_Map", func(t *testing.T) {
		// Listeners still registered when the stream dies must be settled rather than
		// dropped, so anyone blocked in IsFinal is released now instead of waiting out
		// their own context.
		//
		// Note the bound: settlement goes through the same pool as delivery, so listeners
		// that ignore their context can starve it. That is the accepted trade -- nothing
		// can force such a callback to return -- and the teardown deadline is what keeps
		// it from hanging shutdown. See Teardown_Is_Bounded_Regardless_Of_Pending_Listeners.
		t.Parallel()

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = time.Second
		nlm.handlerWorkers = 2

		const pending = 5
		var settled atomic.Int32
		for i := range pending {
			seedHandlers(nlm, "tx_teardown_"+strconv.Itoa(i), &funcListener{
				fn: func(context.Context, string, int, string) { settled.Add(1) },
			})
		}

		ctx, cancel := context.WithCancel(t.Context())
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		listenErr := make(chan error, 1)
		go func() { listenErr <- nlm.listen(ctx) }()
		cancel()
		requireListenReturned(t, listenErr)

		require.Equal(t, int32(pending), settled.Load(),
			"teardown must settle every listener still in the map")
	})

	t.Run("Kept_Callbacks_Are_Retried_With_Local_Expiry_Disabled", func(t *testing.T) {
		// listenerTTL == 0 disables the local expiry backstop -- a documented, supported
		// configuration that leaves the committer's reply as the only thing that settles
		// a listener. The sweeper is also the retry path for callbacks that could not be
		// queued, so it must still run those retries with the backstop off: a deferred
		// listener whose answer already arrived would otherwise never be delivered.
		t.Parallel()

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout
		nlm.handlerWorkers = 2
		nlm.handlerQueueSize = 2 // tiny, so most of the batch is kept
		nlm.listenerTTL = 0      // local expiry disabled
		nlm.sweepInterval = 20 * time.Millisecond

		var valid, unknown atomic.Int32
		listener := &funcListener{fn: func(_ context.Context, _ string, status int, _ string) {
			switch status {
			case fdriver.Valid:
				valid.Add(1)
			case fdriver.Unknown:
				unknown.Add(1)
			}
		}}

		const batch = 24
		ids := make([]string, 0, batch)
		for i := range batch {
			id := "tx_nottl_" + strconv.Itoa(i)
			ids = append(ids, id)
			seedHandlers(nlm, id, listener)
		}

		feedResponses(t.Context(), fakeStream, respFor(ids...))
		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Equal(collect, int32(batch), valid.Load(),
				"every listener must get its real status even with listenerTTL disabled")
		}, 3*time.Second, tick)
		require.Zero(t, unknown.Load(),
			"a committed transaction must never be reported Unknown")
	})

	t.Run("Teardown_Uses_The_Remembered_Status", func(t *testing.T) {
		// A listener still deferred when the stream dies must be settled with the answer
		// the committer already gave, not downgraded to Unknown. The sweeper does this;
		// teardown is the other path that settles from the map and must agree.
		t.Parallel()

		nlm, _ := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout

		var valid, unknown atomic.Int32
		listener := &funcListener{fn: func(_ context.Context, _ string, status int, _ string) {
			switch status {
			case fdriver.Valid:
				valid.Add(1)
			case fdriver.Unknown:
				unknown.Add(1)
			}
		}}

		// tx_committed was answered but could not be handed off; tx_silent never heard back.
		seedHandlers(nlm, "tx_committed", listener)
		seedHandlers(nlm, "tx_silent", listener)
		answered := fdriver.Valid
		nlm.handlersMu.Lock()
		nlm.handlers["tx_committed"].status = &answered
		nlm.handlersMu.Unlock()

		// settleAllAndClear only queues; in listen() the pool drains it. Do that here.
		q := make(chan handlerCall, 8)
		nlm.settleAllAndClear(q, fdriver.Unknown)
		close(q)
		for c := range q {
			nlm.callHandler(t.Context(), c)
		}

		require.Equal(t, int32(1), valid.Load(),
			"the listener whose answer already arrived must be settled with it, not Unknown")
		require.Equal(t, int32(1), unknown.Load(),
			"the listener nothing was heard for must still be settled Unknown")
	})

	t.Run("Saturated_Queue_Settles_Every_Listener_Exactly_Once", func(t *testing.T) {
		// The property the whole deferral machinery exists for: with the queue far too
		// small for the batch, every listener is still settled, with the status the
		// committer actually sent, and never twice.
		//
		// Several listeners per txID on purpose. Exactly-once needs the hand-off to be
		// all-or-nothing: a per-listener best-effort send would queue some of an entry's
		// listeners and defer the entry anyway, and the retry -- which only knows the
		// entry, not which of its listeners already ran -- would settle those a second
		// time. Pairing the hand-off with the delete is what rules that out.
		t.Parallel()

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout
		nlm.handlerWorkers = 2
		// Small enough to saturate, but not smaller than one entry's listener count:
		// an entry that cannot fit even into an empty queue is never handed off at all.
		nlm.handlerQueueSize = 4
		nlm.listenerTTL = 0 // no local backstop: only redelivery can settle these
		nlm.sweepInterval = 10 * time.Millisecond

		const txCount = 20
		const perTx = 3

		var mu sync.Mutex
		seen := make(map[string][]int, txCount*perTx)

		ids := make([]string, 0, txCount)
		for i := range txCount {
			id := "tx_exactly_once_" + strconv.Itoa(i)
			ids = append(ids, id)
			for j := range perTx {
				key := id + "#" + strconv.Itoa(j)
				seedListener(nlm, id, &funcListener{
					fn: func(_ context.Context, _ string, status int, _ string) {
						// Slow enough that the queue genuinely saturates rather than the
						// workers keeping pace with the dispatcher.
						time.Sleep(200 * time.Microsecond)
						mu.Lock()
						defer mu.Unlock()
						seen[key] = append(seen[key], status)
					},
				})
			}
		}

		feedResponses(t.Context(), fakeStream, respFor(ids...)) // every txID COMMITTED
		runManager(t, nlm)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			mu.Lock()
			defer mu.Unlock()
			assert.Len(collect, seen, txCount*perTx)
		}, 3*time.Second, tick, "not every listener was settled")

		mu.Lock()
		defer mu.Unlock()
		for i := range txCount {
			for j := range perTx {
				key := "tx_exactly_once_" + strconv.Itoa(i) + "#" + strconv.Itoa(j)
				require.Equal(t, []int{fdriver.Valid}, seen[key],
					"listener %s must be settled exactly once, with the status the committer sent", key)
			}
		}
	})
}

func TestHandOff(t *testing.T) {
	t.Parallel()

	noop := &funcListener{fn: func(context.Context, string, int, string) {}}
	listeners := func(n int) []fabric.FinalityListener {
		out := make([]fabric.FinalityListener, n)
		for i := range out {
			out[i] = noop
		}
		return out
	}

	t.Run("Queues_The_Whole_Entry_When_It_Fits", func(t *testing.T) {
		t.Parallel()

		q := make(chan handlerCall, 4)
		q <- handlerCall{} // one slot already taken

		require.True(t, handOff(q, "tx", listeners(3), fdriver.Valid))
		require.Len(t, q, 4, "all three listeners should have been queued")
	})

	t.Run("Queues_Nothing_When_The_Entry_Does_Not_Fit", func(t *testing.T) {
		// All-or-nothing is what makes exactly-once delivery possible: the retry path
		// only knows the entry, not which of its listeners already ran, so a partial
		// hand-off would settle those a second time on the next sweep.
		t.Parallel()

		q := make(chan handlerCall, 4)
		q <- handlerCall{}
		q <- handlerCall{} // two free slots, entry needs three

		require.False(t, handOff(q, "tx", listeners(3), fdriver.Valid))
		require.Len(t, q, 2, "a rejected entry must leave nothing behind in the queue")
	})

	t.Run("Never_Blocks_On_A_Full_Queue", func(t *testing.T) {
		// The caller is the dispatcher goroutine, which also drives the sweeper:
		// blocking here would stall both.
		t.Parallel()

		q := make(chan handlerCall, 1)
		q <- handlerCall{}

		done := make(chan bool, 1)
		go func() { done <- handOff(q, "tx", listeners(1), fdriver.Valid) }()

		select {
		case queued := <-done:
			require.False(t, queued)
		case <-time.After(time.Second):
			t.Fatal("handOff blocked on a full queue")
		}
	})
}
