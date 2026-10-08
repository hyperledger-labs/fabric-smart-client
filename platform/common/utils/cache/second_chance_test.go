/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecondChanceTouchedEntrySurvives(t *testing.T) {
	t.Parallel()
	c := NewSecondChanceCache[string, string](2, 1, nil)

	c.Put("a", "1")
	c.Put("b", "2")

	// Read a, so only b is unreferenced when the cache fills.
	v, ok := c.Get("a")
	require.True(t, ok)
	assert.Equal(t, "1", v)

	c.Put("c", "3")

	_, ok = c.Get("b")
	assert.False(t, ok, "unreferenced entry should have been the victim")

	v, ok = c.Get("a")
	require.True(t, ok, "entry read before the scan should survive it")
	assert.Equal(t, "1", v)

	v, ok = c.Get("c")
	require.True(t, ok)
	assert.Equal(t, "3", v)
}

func TestSecondChanceUntouchedEntryEvicted(t *testing.T) {
	t.Parallel()
	c := NewSecondChanceCache[string, string](2, 1, nil)

	c.Put("a", "1")
	c.Put("b", "2")
	c.Put("c", "3")

	_, ok := c.Get("a")
	assert.False(t, ok, "oldest unreferenced entry should be evicted first")
	assert.Equal(t, 2, c.Len())
}

func TestSecondChanceHoldsSizeUnderChurn(t *testing.T) {
	t.Parallel()
	const size = 8
	c := NewSecondChanceCache[int, int](size, 3, nil)

	for i := range 200 {
		c.Put(i, i)
		assert.LessOrEqual(t, c.Len(), size, "cache exceeded its size bound at insertion %d", i)
	}
}

func TestSecondChanceOnEvictReceivesVictims(t *testing.T) {
	t.Parallel()
	var evicted map[string]string
	c := NewSecondChanceCache(2, 1, func(m map[string]string) { evicted = m })

	c.Put("a", "1")
	c.Put("b", "2")
	c.Put("c", "3")

	require.Len(t, evicted, 1)
	assert.Equal(t, "1", evicted["a"])
}

// A deleted key leaves its slot behind, since evictionCache.Delete does not
// notify the policy. The scan must reclaim that slot rather than give it a
// second chance, and must not report the key as evicted.
func TestSecondChanceReclaimsSlotOfDeletedKey(t *testing.T) {
	t.Parallel()
	var evicted map[string]string
	c := NewSecondChanceCache(2, 1, func(m map[string]string) { evicted = m })

	c.Put("a", "1")
	c.Put("b", "2")
	c.Delete("a")

	c.Put("c", "3")

	assert.Empty(t, evicted, "a stale slot is not an eviction")

	v, ok := c.Get("b")
	require.True(t, ok, "live entry must not be evicted through a stale slot")
	assert.Equal(t, "2", v)

	v, ok = c.Get("c")
	require.True(t, ok)
	assert.Equal(t, "3", v)
}

// A key deleted and added again must reuse its slot, not claim a second one.
func TestSecondChanceReaddedKeyReusesSlot(t *testing.T) {
	t.Parallel()
	c := NewSecondChanceCache[string, string](3, 1, nil)

	c.Put("a", "1")
	c.Delete("a")
	c.Put("a", "2")

	c.Put("b", "3")
	c.Put("c", "4")

	v, ok := c.Get("a")
	require.True(t, ok)
	assert.Equal(t, "2", v)
	assert.Equal(t, 3, c.Len())
}

func TestSecondChanceSizeOne(t *testing.T) {
	t.Parallel()
	c := NewSecondChanceCache[string, string](1, 1, nil)

	c.Put("a", "1")
	v, ok := c.Get("a")
	require.True(t, ok)
	assert.Equal(t, "1", v)

	c.Put("b", "2")
	_, ok = c.Get("a")
	assert.False(t, ok)
	v, ok = c.Get("b")
	require.True(t, ok)
	assert.Equal(t, "2", v)
}

func TestSecondChanceConcurrent(t *testing.T) {
	t.Parallel()
	c := NewSecondChanceCache[string, string](25, 8, nil)

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := range workers {
		go func() {
			defer wg.Done()
			for j := range 2000 {
				key := fmt.Sprintf("key-%d-%d", i, j)
				c.Put(key, key)
				if v, ok := c.Get(key); ok {
					assert.Equal(t, key, v)
				}
				c.Get("shared")
				c.Put("shared", "shared")
			}
		}()
	}
	wg.Wait()
}
