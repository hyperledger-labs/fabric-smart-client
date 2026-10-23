/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/cache"
)

// benchTxIDs returns a fixed pool of transaction IDs, so the benchmark loops
// measure the cache rather than string formatting.
func benchTxIDs(n int) []driver.TxID {
	ids := make([]driver.TxID, n)
	for i := range ids {
		ids[i] = "tx" + strconv.Itoa(i)
	}
	return ids
}

// newBenchStore builds a cachedStore over a second-chance cache of the given
// size and eviction buffer, the way NewCachedVault does.
func newBenchStore(size, buffer int) (*cachedStore, *mockVaultStore) {
	backed := newMockVaultStore()
	return &cachedStore{
		VaultStore: backed,
		cache:      cache.NewSecondChanceCache[driver.TxID, *entry](size, buffer, nil),
	}, backed
}

// reportHitRate reports hits per cache read: 1.0 means every read was served
// from the cache, 0.0 means every read fell through to the backing store. A
// miss costs exactly one backing GetTxStatus, so hits are reads minus backing
// calls.
func reportHitRate(b *testing.B, reads int, backingCalls int64) {
	b.Helper()
	if reads == 0 {
		return
	}
	b.ReportMetric(float64(reads-int(backingCalls))/float64(reads), "hits/op")
}

// BenchmarkCachedStore measures the vault tx status cache across eviction
// buffer sizes. The buffer is how many slots a victim scan frees: a buffer of 1
// keeps a full cache full, while a larger buffer makes each scan cheaper per
// insert but leaves the cache partly empty afterwards.
//
// hits/op is the metric that answers the question. The backing store here is a
// map lookup, so a miss costs almost nothing; against a real database a miss is
// orders of magnitude more expensive than any ns/op difference between buffers.
func BenchmarkCachedStore(b *testing.B) {
	// 100 is the default for vault.txidstore.cache.size; 10000 stands in for a
	// busy node.
	for _, size := range []int{100, 10000} {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for _, buffer := range []int{1, max(size/10, 1), max(size/3, 1)} {
				b.Run(fmt.Sprintf("buffer=%d", buffer), func(b *testing.B) {
					b.Run("hotset", func(b *testing.B) { benchHotSet(b, size, buffer) })
					b.Run("streaming", func(b *testing.B) { benchStreaming(b, size, buffer) })
					b.Run("mixed", func(b *testing.B) { benchMixed(b, size, buffer) })
					b.Run("hotset-parallel", func(b *testing.B) { benchHotSetParallel(b, size, buffer) })
				})
			}
		})
	}
}

// reReadLag returns how far back iteration i re-reads. It sweeps from a quarter
// of the cache to a little past it, so hits/op reports what fraction of that
// range the cache still holds. A fixed distance would not do: a miss re-inserts
// through forceGet, which doubles the churn and pushes the next read of the same
// distance out too, so the metric latches at all-hit or all-miss.
func reReadLag(i, size int) int {
	minLag, maxLag := max(size/4, 1), max(size*5/4, 2)
	return minLag + (i*7919)%(maxLag-minLag)
}

// benchHotSet reads a small set of recent txIDs repeatedly while new txIDs keep
// arriving through SetStatuses, so the cache is under constant eviction
// pressure while callers keep asking for the hot set.
func benchHotSet(b *testing.B, size, buffer int) {
	b.Helper()
	s, backed := newBenchStore(size, buffer)
	ctx := context.Background()
	hot := max(size/10, 1)
	ids := benchTxIDs(size * 8)

	for i := range hot {
		if err := s.SetStatuses(ctx, driver.Valid, "", ids[i]); err != nil {
			b.Fatal(err)
		}
	}

	reads := 0
	for i := 0; b.Loop(); i++ {
		// One new txID arrives for every four reads of the hot set.
		if i%5 == 4 {
			if err := s.SetStatuses(ctx, driver.Valid, "", ids[hot+(i/5)%(len(ids)-hot)]); err != nil {
				b.Fatal(err)
			}
			continue
		}
		if _, err := s.GetTxStatus(ctx, ids[i%hot]); err != nil {
			b.Fatal(err)
		}
		reads++
	}
	reportHitRate(b, reads, backed.getCalls.Load())
}

// benchStreaming writes each txID once and reads it back twice, the shape of a
// node following transactions as they commit. The first read follows the write
// directly and always hits; only the second, reReadLag back, depends on the
// cache still holding recent entries, which is what a large buffer gives up.
func benchStreaming(b *testing.B, size, buffer int) {
	b.Helper()
	s, backed := newBenchStore(size, buffer)
	ctx := context.Background()
	ids := benchTxIDs(size * 8)

	reads := 0
	for i := 0; b.Loop(); i++ {
		id := ids[i%len(ids)]
		if err := s.SetStatuses(ctx, driver.Valid, "", id); err != nil {
			b.Fatal(err)
		}
		if _, err := s.GetTxStatus(ctx, id); err != nil {
			b.Fatal(err)
		}
		reads++
		lag := reReadLag(i, size)
		if i < lag {
			continue
		}
		if _, err := s.GetTxStatus(ctx, ids[(i-lag)%len(ids)]); err != nil {
			b.Fatal(err)
		}
		reads++
	}
	reportHitRate(b, reads, backed.getCalls.Load())
}

// benchMixed is the streaming shape with Invalidate on one txID in eight, which
// is what the vault does when it must re-read a transaction from the store.
// Invalidate deletes the entry outright, so the slot it leaves behind is stale
// until a victim scan reclaims it.
func benchMixed(b *testing.B, size, buffer int) {
	b.Helper()
	s, backed := newBenchStore(size, buffer)
	ctx := context.Background()
	ids := benchTxIDs(size * 8)

	reads := 0
	for i := 0; b.Loop(); i++ {
		id := ids[i%len(ids)]
		if err := s.SetStatuses(ctx, driver.Valid, "", id); err != nil {
			b.Fatal(err)
		}
		if i%8 == 0 {
			s.Invalidate(id)
		}
		lag := reReadLag(i, size)
		if i < lag {
			continue
		}
		if _, err := s.GetTxStatus(ctx, ids[(i-lag)%len(ids)]); err != nil {
			b.Fatal(err)
		}
		reads++
	}
	reportHitRate(b, reads, backed.getCalls.Load())
}

// benchHotSetParallel runs the hot-set reads concurrently, since GetTxStatus is
// called from many goroutines in a running node. It measures contention, so
// only its ns/op compares across buffers: one read in five is of a cold txID
// from a pool far larger than the cache, so hits/op sits at 0.8 by design.
//
// All goroutines draw from one shared sequence. With a per-goroutine index they
// would request the same cold txIDs in lockstep, and one goroutine's miss would
// turn into hits for the others, inflating hits/op with GOMAXPROCS.
//
// mockVaultStore.statuses is a plain map, so this pattern cannot write through
// SetStatuses while readers run without racing inside the mock itself. The
// backing store is filled directly up front instead; the cold reads still drive
// misses, Puts and eviction concurrently. Giving the mock a mutex would put it
// in the measurement path.
func benchHotSetParallel(b *testing.B, size, buffer int) {
	b.Helper()
	s, backed := newBenchStore(size, buffer)
	ctx := context.Background()
	hot := max(size/10, 1)
	ids := benchTxIDs(size * 8)

	if err := backed.SetStatuses(ctx, driver.Valid, "", ids...); err != nil {
		b.Fatal(err)
	}
	// Warm the cache with the hot set only.
	for i := range hot {
		if _, err := s.GetTxStatus(ctx, ids[i]); err != nil {
			b.Fatal(err)
		}
	}

	var seq atomic.Int64
	backed.getCalls.Store(0)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := int(seq.Add(1) - 1)
			id := ids[i%hot]
			if i%5 == 4 {
				id = ids[hot+i%(len(ids)-hot)]
			}
			if _, err := s.GetTxStatus(ctx, id); err != nil {
				b.Error(err)
				return
			}
		}
	})
	reportHitRate(b, int(seq.Load()), backed.getCalls.Load())
}
