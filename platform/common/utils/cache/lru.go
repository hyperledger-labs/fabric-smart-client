/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import "sync"

// NewLRUCache creates a cache with limited size with LRU eviction policy
func NewLRUCache[K comparable, V any](size, buffer int, onEvict func(map[K]V)) *evictionCache[K, V] {
	m := map[K]V{}
	return &evictionCache[K, V]{
		m:              m,
		l:              &noLock{},
		evictionPolicy: NewLRUEviction(size, buffer, func(keys []K) { evict(keys, m, onEvict) }),
	}
}

func NewLRUEviction[K comparable](size, buffer int, evict func([]K)) *lruEviction[K] {
	return &lruEviction[K]{
		size:  size,
		cap:   size + buffer,
		keys:  make([]K, 0, size+buffer),
		evict: evict,
	}
}

type lruEviction[K comparable] struct {
	// size is the minimum amount of entries guaranteed to be kept in cache.
	size int
	// cap + size is the maximum amount of entries that can be kept in cache. After that, a cleanup is invoked.
	cap int
	// keys keeps track of which keys should be evicted.
	// The last element of the slice is the most recent one.
	// Performance improvement: keep sliding index to avoid reallocating
	keys []K
	// evict is called when we evict
	evict func([]K)
	mu    sync.Mutex
}

func (c *lruEviction[K]) Push(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keys = append(c.keys, key)
	if len(c.keys) <= c.cap {
		return
	}
	logger.Infof("Capacity of %d exceeded. Evicting old keys by shifting LRU keys keeping only the %d most recent ones", c.cap, c.size)
	c.evict(c.keys[0 : c.cap-c.size])
	c.keys = (c.keys)[c.cap-c.size:]
}
