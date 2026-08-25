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
		// A saturated pool must not stall the dispatcher: it keeps draining
		// responseQueue and keeps sweeping. Once the queue fills, further calls are
		// dropped with a warning rather than blocking.
		t.Parallel()

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout
		nlm.handlerWorkers = 1
		// Tiny queue so the flood below actually saturates it.
		nlm.callQueue = make(chan handlerCall, 2)

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
			assert.Equal(collect, int32(1), blocker.inFlight.Load())
		}, timeout, tick, "the single handler slot should be held")

		// The dispatcher must have finished the whole batch (all entries removed
		// from the map) even though almost all of it was dropped.
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			nlm.handlersMu.RLock()
			remaining := len(nlm.handlers)
			nlm.handlersMu.RUnlock()
			assert.Zero(collect, remaining,
				"dispatcher must drain the batch even when the queue is full")
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
		// Why the queue exists. One notification response can carry far more
		// transactions than there are workers, dispatched in a tight loop; without a
		// buffer everything past the worker count is dropped even though the listeners
		// are healthy. Concurrency must still be capped -- see peak below.
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
		// select picks at random among ready cases, so a worker can take the queue
		// branch after cancellation; passing the cancelled context there gives the
		// listener an already-expired timeout.
		//
		// Primes the queue and cancels before the workers start so their first select
		// has both cases ready -- a normal run does not reliably hit this.
		t.Parallel()

		const queued = 60

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout
		nlm.handlerWorkers = 4
		nlm.callQueue = make(chan handlerCall, queued)

		var live, dead atomic.Int32
		listener := &funcListener{fn: func(ctx context.Context, _ string, _ int, _ string) {
			if ctx.Err() != nil {
				dead.Add(1)
			} else {
				live.Add(1)
			}
		}}

		// Work already accepted, awaiting a worker.
		for i := range queued {
			nlm.callQueue <- handlerCall{
				handler: listener,
				txID:    "tx_cancelctx_" + strconv.Itoa(i),
				status:  fdriver.Valid,
			}
		}

		ctx, cancel := context.WithCancel(t.Context())
		fakeStream.RecvStub = func() (*committerpb.NotificationResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		listenErr := make(chan error, 1)
		go func() { listenErr <- nlm.listen(ctx) }()
		cancel() // race the workers' first select

		requireListenReturned(t, listenErr)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Equal(collect, int32(queued), live.Load()+dead.Load(),
				"every queued callback should have run")
		}, timeout, tick)

		require.Zero(t, dead.Load(),
			"%d of %d callbacks were handed an already-cancelled context; they must never see one",
			dead.Load(), queued)
	})

	t.Run("Teardown_Settles_At_Worker_Concurrency", func(t *testing.T) {
		// Teardown must settle at handlerWorkers concurrency, not one at a time:
		// serially, N stuck listeners block listen() for N*handlerTimeout.
		t.Parallel()

		const stuck = 12
		const workers = 6
		const hTimeout = 100 * time.Millisecond

		nlm, _ := setupTest(t)
		nlm.handlerTimeout = hTimeout
		nlm.handlerWorkers = workers

		block := make(chan struct{}) // never closed: every listener hangs
		t.Cleanup(func() { close(block) })
		for i := range stuck {
			seedHandlers(nlm, "tx_settle_"+strconv.Itoa(i), &funcListener{
				fn: func(context.Context, string, int, string) { <-block },
			})
		}

		start := time.Now()
		nlm.settleAllAndClear(t.Context(), fdriver.Unknown)
		elapsed := time.Since(start)

		// stuck/workers batches plus slack; serial would be ~12x hTimeout.
		require.Less(t, elapsed, time.Duration(stuck/workers+2)*hTimeout,
			"settling %d stuck listeners took %s: teardown is serial rather than %d at a time",
			stuck, elapsed, workers)
	})

	t.Run("Teardown_Drains_Queued_Callbacks", func(t *testing.T) {
		// Callbacks already sitting in callQueue when the stream stops must still be
		// delivered. dispatch has removed their handlers entries by then, so
		// settleAllAndClear will not settle them: dropping them here loses the
		// notification outright, with no Unknown fallback.
		t.Parallel()

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = testHandlerTimeout
		// One slow worker, so the batch backs up in the queue rather than draining
		// as fast as it is dispatched.
		nlm.handlerWorkers = 1
		nlm.callQueue = make(chan handlerCall, 200)

		var ran atomic.Int32
		slow := &funcListener{fn: func(context.Context, string, int, string) {
			ran.Add(1)
			time.Sleep(20 * time.Millisecond)
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

		// Let the batch reach the queue, but not be drained.
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Positive(collect, len(nlm.callQueue), "batch should be buffered")
		}, timeout, tick)

		cancel()
		requireListenReturned(t, listenErr)

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Equal(collect, int32(batch), ran.Load())
		}, timeout, tick,
			"every queued callback must be delivered on teardown, not discarded with the workers")
	})

	t.Run("Teardown_Settles_Listeners_While_Slot_Held", func(t *testing.T) {
		// Listeners left in the map when the stream dies must be settled, even
		// though the pool's workers may be held by stuck callbacks -- which
		// is why that path invokes them directly rather than via the queue.
		t.Parallel()

		nlm, fakeStream := setupTest(t)
		nlm.handlerTimeout = 100 * time.Millisecond
		// One slot, and it will be held forever by the stuck callback below.
		nlm.handlerWorkers = 1

		// Derived from t.Context() so the test's own deadline or failure also
		// tears this down; cancel() is what drives the teardown under test.
		ctx, cancel := context.WithCancel(t.Context())

		stuck := newCountingBlockingListener()
		t.Cleanup(func() { close(stuck.block) })
		seedHandlers(nlm, "tx_holds_the_only_slot", stuck)

		// These are still registered at teardown, when the only slot is held.
		const pending = 5
		var settled atomic.Int32
		var wg sync.WaitGroup
		wg.Add(pending)
		for i := range pending {
			seedHandlers(nlm, "tx_teardown_"+strconv.Itoa(i), &funcListener{
				fn: func(context.Context, string, int, string) {
					settled.Add(1)
					wg.Done()
				},
			})
		}

		feedResponses(ctx, fakeStream, respFor("tx_holds_the_only_slot"))

		listenErr := make(chan error, 1)
		go func() { listenErr <- nlm.listen(ctx) }()

		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			assert.Equal(collect, int32(1), stuck.inFlight.Load())
		}, timeout, tick, "the stuck callback should hold the only slot")

		cancel()
		requireListenReturned(t, listenErr)

		finished := make(chan struct{})
		go func() { wg.Wait(); close(finished) }()
		select {
		case <-finished:
		case <-time.After(timeout):
			t.Fatalf("teardown settled only %d of %d listeners while a callback held the only slot",
				settled.Load(), pending)
		}
	})
}
