/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// NewSecondChanceCache creates a cache with limited size with second-chance
// (CLOCK) eviction, an approximation of LRU that promotes an entry on every read
// rather than on insertion.
//
// Unlike NewLRUCache, size is the number of entries the cache holds, not a
// guaranteed minimum: once it is full, the next insertion scans for victims and
// evicts buffer of them. buffer is clamped to [1, size]. A larger buffer runs
// the scan, and onEvict, less often. onEvict runs under the cache's write lock
// and must not call back into the cache.
//
//nolint:revive // unexported-return: mirrors NewLRUCache; evictionCache is the shared implementation and exporting it is an API change
func NewSecondChanceCache[K comparable, V any](size, buffer int, onEvict func(map[K]V)) *evictionCache[K, V] {
	m := map[K]V{}
	return &evictionCache[K, V]{
		m: m,
		l: &sync.RWMutex{},
		evictionPolicy: newSecondChanceEviction(
			size, buffer,
			func(key K) bool { _, ok := m[key]; return ok },
			func(keys []K) { evict(keys, m, onEvict) },
		),
	}
}

// newSecondChanceEviction returns a second-chance policy over size slots,
// freeing buffer of them per victim scan.
//
// present reports whether a key is still held by the cache. evictionCache.Delete
// does not notify the policy, so a slot can outlive its entry; the scan uses this
// to reclaim such a slot instead of spending a second chance on it.
//
// The policy takes no lock of its own. It relies on the EvictionPolicy contract,
// which evictionCache upholds through its RWMutex: Push runs alone, while Touch
// and String may run concurrently with each other.
func newSecondChanceEviction[K comparable](size, buffer int, present func(K) bool, evict func([]K)) *secondChanceEviction[K] {
	size = max(size, 1)
	buffer = min(max(buffer, 1), size)

	free := make([]int, size)
	for i := range free {
		free[i] = size - 1 - i
	}

	return &secondChanceEviction[K]{
		slots:      make([]K, size),
		referenced: make([]atomic.Bool, size),
		keySlot:    make(map[K]int, size),
		free:       free,
		buffer:     buffer,
		present:    present,
		evict:      evict,
	}
}

type secondChanceEviction[K comparable] struct {
	// slots is the ring the victim scan rotates through. A slot is live iff its
	// key maps back to it in keySlot.
	slots []K
	// referenced records whether a slot was read since the last scan passed it.
	// It is atomic because concurrent Touch calls set it.
	referenced []atomic.Bool
	// keySlot locates a key's slot, for Touch and for reuse on re-insertion.
	// Only Push modifies it, directly or through scan.
	keySlot map[K]int
	// free holds slots not currently assigned to a key.
	free []int
	// position is the next slot the victim scan will examine.
	position int
	// buffer is how many slots one scan frees.
	buffer int

	present func(K) bool
	evict   func([]K)
}

func (c *secondChanceEviction[K]) Push(key K) {
	// Re-inserting a key reuses its slot rather than claiming a second one, so a
	// key deleted from the cache and added again cannot be evicted through a stale slot.
	if i, ok := c.keySlot[key]; ok {
		c.referenced[i].Store(true)
		return
	}

	if len(c.free) == 0 {
		c.scan()
	}

	i := c.free[len(c.free)-1]
	c.free = c.free[:len(c.free)-1]
	c.slots[i] = key
	c.referenced[i].Store(false)
	c.keySlot[key] = i
}

// Touch marks a key as recently used. It runs on every cache hit and takes no
// lock; see newSecondChanceEviction.
func (c *secondChanceEviction[K]) Touch(key K) {
	if i, ok := c.keySlot[key]; ok {
		c.referenced[i].Store(true)
	}
}

// scan frees buffer slots, clearing the referenced bit of every slot it passes
// over. Push calls it only when no slot is free, so every slot is live when the
// scan starts.
func (c *secondChanceEviction[K]) scan() {
	size := len(c.slots)
	evicted := make([]K, 0, c.buffer)

	for freed := 0; freed < c.buffer; {
		i := c.position
		c.position = (c.position + 1) % size

		key := c.slots[i]
		if _, ok := c.keySlot[key]; !ok {
			// Freed earlier in this scan, which wrapped around the ring.
			continue
		}
		stale := !c.present(key)

		if !stale && c.referenced[i].Load() {
			// Spend the second chance: this entry survives the pass.
			c.referenced[i].Store(false)
			continue
		}

		delete(c.keySlot, key)
		c.referenced[i].Store(false)
		c.free = append(c.free, i)
		freed++

		if !stale {
			evicted = append(evicted, key)
		}
	}

	if len(evicted) > 0 {
		c.evict(evicted)
	}
}

func (c *secondChanceEviction[K]) String() string {
	referenced := make([]bool, len(c.referenced))
	for i := range c.referenced {
		referenced[i] = c.referenced[i].Load()
	}
	return fmt.Sprintf("Slots: [%v], Referenced: [%v], Position: [%d]", c.slots, referenced, c.position)
}
