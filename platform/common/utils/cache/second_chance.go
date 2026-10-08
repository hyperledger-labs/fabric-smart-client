/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	"fmt"
	"sync"
)

// NewSecondChanceCache creates a cache with limited size with second-chance
// (CLOCK) eviction, an approximation of LRU that promotes an entry on every read
// rather than on insertion.
//
// Unlike NewLRUCache, size is the number of entries the cache holds, not a
// guaranteed minimum: once it is full, the next insertion scans for victims and
// evicts buffer of them. buffer is clamped to [1, size].
//
//nolint:revive // unexported-return: mirrors NewLRUCache; evictionCache is the shared implementation and exporting it is an API change
func NewSecondChanceCache[K comparable, V any](size, buffer int, onEvict func(map[K]V)) *evictionCache[K, V] {
	m := map[K]V{}
	return &evictionCache[K, V]{
		m: m,
		l: &sync.RWMutex{},
		evictionPolicy: NewSecondChanceEviction(
			size, buffer,
			func(key K) bool { _, ok := m[key]; return ok },
			func(keys []K) { evict(keys, m, onEvict) },
		),
	}
}

// NewSecondChanceEviction returns a second-chance policy over size slots,
// freeing buffer of them per victim scan.
//
// present reports whether a key is still held by the cache. evictionCache.Delete
// does not notify the policy, so a slot can outlive its entry; the scan uses this
// to reclaim such a slot instead of spending a second chance on it.
//
//nolint:revive // unexported-return: mirrors NewLRUEviction; the policy is consumed through the EvictionPolicy interface
func NewSecondChanceEviction[K comparable](size, buffer int, present func(K) bool, evict func([]K)) *secondChanceEviction[K] {
	size = max(size, 1)
	buffer = min(max(buffer, 1), size)

	free := make([]int, size)
	for i := range free {
		free[i] = size - 1 - i
	}

	return &secondChanceEviction[K]{
		slots:      make([]K, size),
		referenced: make([]bool, size),
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
	referenced []bool
	// keySlot locates a key's slot, for Touch and for reuse on re-insertion.
	keySlot map[K]int
	// free holds slots not currently assigned to a key.
	free []int
	// position is the next slot the victim scan will examine.
	position int
	// buffer is how many slots one scan frees.
	buffer int

	present func(K) bool
	evict   func([]K)

	mu sync.Mutex
}

func (c *secondChanceEviction[K]) Push(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Re-inserting a key reuses its slot rather than claiming a second one, so a
	// key deleted from the cache and added again cannot be evicted through a stale slot.
	if i, ok := c.keySlot[key]; ok {
		c.referenced[i] = true
		return
	}

	if len(c.free) == 0 {
		c.scan()
	}

	i := c.free[len(c.free)-1]
	c.free = c.free[:len(c.free)-1]
	c.slots[i] = key
	c.referenced[i] = false
	c.keySlot[key] = i
}

// Touch marks a key as recently used. Get holds only a read lock on the cache,
// so this takes the policy's own lock.
func (c *secondChanceEviction[K]) Touch(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if i, ok := c.keySlot[key]; ok {
		c.referenced[i] = true
	}
}

// scan frees buffer slots, clearing the referenced bit of every slot it passes
// over. Callers must hold c.mu and must have found no free slot.
func (c *secondChanceEviction[K]) scan() {
	size := len(c.slots)
	evicted := make([]K, 0, c.buffer)

	for freed := 0; freed < c.buffer; {
		i := c.position
		c.position = (c.position + 1) % size

		key := c.slots[i]
		stale := !c.present(key)

		if !stale && c.referenced[i] {
			// Spend the second chance: this entry survives the pass.
			c.referenced[i] = false
			continue
		}

		delete(c.keySlot, key)
		c.referenced[i] = false
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
	c.mu.Lock()
	defer c.mu.Unlock()
	return fmt.Sprintf("Slots: [%v], Referenced: [%v], Position: [%d]", c.slots, c.referenced, c.position)
}
