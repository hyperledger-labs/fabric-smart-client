/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/cache"
)

// stableGoroutineCount runs the GC and gives the scheduler a moment to settle so
// that runtime.NumGoroutine() reflects goroutines that are genuinely still alive
// (parked/running), not ones about to be reaped.
func stableGoroutineCount() int {
	var last int
	for range 10 {
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

func TestTimeoutCache_CleanupGoroutineStopsOnCancel(t *testing.T) { //nolint:paralleltest // measures process-wide goroutine counts; must run serially
	before := stableGoroutineCount()

	ctx, cancel := context.WithCancel(context.Background())
	const n = 50
	for range n {
		c := cache.NewTimeoutCache[int, int](ctx, time.Hour, func(map[int]int) {})
		c.Put(1, 1)
	}
	// Goroutines exist while ctx is live.
	require.GreaterOrEqual(t, stableGoroutineCount()-before, n-5)

	// Cancelling the context must stop every cleanup goroutine.
	cancel()
	require.Eventually(t, func() bool {
		return stableGoroutineCount()-before <= 2
	}, 5*time.Second, 50*time.Millisecond,
		"cleanup goroutines must exit after ctx cancel; see fsc-memory.md Issue #1")
}
