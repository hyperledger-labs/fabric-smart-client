/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stableGoroutineCount runs the GC and gives the scheduler a moment to settle so
// that runtime.NumGoroutine() reflects goroutines that are genuinely still alive.
func stableGoroutineCount() int {
	var last int
	for i := 0; i < 10; i++ {
		runtime.GC()
		time.Sleep(20 * time.Millisecond)
		n := runtime.NumGoroutine()
		if n == last {
			return n
		}
		last = n
	}
	return last
}

// TestBatcher_StartGoroutineStopsOnCancel verifies fsc-memory.md Issue #5 is fixed.
//
// newBatcher spawns `go e.start()`, whose body is an unconditional `for { select {...} }`.
// The batcher now takes a context.Context: cancelling it must make start() return (and
// stop its ticker) so NewBatchExecutor / NewBatchRunner no longer leak a goroutine per
// call for the life of the process.
func TestBatcher_StartGoroutineStopsOnCancel(t *testing.T) { //nolint:paralleltest // measures process-wide goroutine counts; must run serially
	before := stableGoroutineCount()

	ctx, cancel := context.WithCancel(context.Background())
	const n = 50
	for range n {
		r := NewBatchRunner(ctx, func(vs []int) []error { return make([]error, len(vs)) }, 10, time.Hour)
		_ = r
	}
	require.GreaterOrEqual(t, stableGoroutineCount()-before, n-5)

	cancel()
	require.Eventually(t, func() bool {
		return stableGoroutineCount()-before <= 2
	}, 5*time.Second, 50*time.Millisecond,
		"batcher start goroutines must exit after ctx cancel; see fsc-memory.md Issue #5")
}

// TestBatcher_StopsOnCancelWithInflightCall verifies that cancelling ctx while start()
// is parked on one of its INTERIOR blocking channel operations (not the top-of-loop
// select, which was already guarded) still makes start() return.
//
// start() has interior receives/sends after the top-of-loop select: the ticker-branch
// lastElement read, the drain loop that reads the rest of the batch, and the
// output-distribution send. If any of these is not guarded with its own
// `case <-r.ctx.Done(): return`, a cancel landing while start() is blocked there leaves
// the goroutine parked forever, reintroducing the leak.
//
// A single in-flight Run() with capacity > 1 is not enough to reach these interior ops:
// its send simply blocks in call()'s own (already-guarded) select on a slot start() isn't
// watching yet, so start() never leaves the top-of-loop select. To genuinely park start()
// inside the drain loop we need a "full cycle" to fire (a send to the batch's last slot)
// while slots before it are, and forever remain, unfilled. We do this deterministically,
// same-package white-box, by presetting the batcher's sequence counter so the single
// in-flight call lands directly on the last slot: start()'s top select immediately sees a
// full cycle and moves into the drain loop expecting inputs for the earlier slots — slots
// that no call() will ever fill, since only this one call is ever made.
func TestBatcher_StopsOnCancelWithInflightCall(t *testing.T) { //nolint:paralleltest // measures process-wide goroutine counts; must run serially
	before := stableGoroutineCount()

	ctx, cancel := context.WithCancel(context.Background())
	const capacity = 2
	br := NewBatchRunner(ctx, func(vs []int) []error { return make([]error, len(vs)) }, capacity, time.Hour)
	b, ok := br.(*batchRunner[int])
	require.True(t, ok, "expected NewBatchRunner to return *batchRunner[int]")

	// Force the single call about to be made to land on the last slot of the first
	// cycle, so start()'s top-of-loop select fires immediately on it and enters the
	// drain loop, which then blocks forever waiting for slot 0 (nobody will ever send
	// there) unless the interior guard is present.
	atomic.StoreUint32(&b.idx, uint32(capacity-1))

	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_ = br.Run(1)
	}()

	// Give start() time to receive the last-slot input and genuinely park inside the
	// drain loop, waiting on a slot with no sender.
	time.Sleep(200 * time.Millisecond)

	cancel()

	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight Run() did not return after ctx cancel")
	}

	require.Eventually(t, func() bool {
		return stableGoroutineCount()-before <= 1
	}, 5*time.Second, 50*time.Millisecond,
		"start() goroutine must exit even when cancel lands on an interior blocking op with no counterpart")
}
