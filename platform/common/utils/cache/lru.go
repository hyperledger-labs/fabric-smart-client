/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	"fmt"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
)

// NewLRUCache creates a cache with limited size with LRU eviction policy.
// It is guaranteed that at least size elements can be kept in the cache.
// The cache is cleaned up when the cache is full, i.e. it contains size + buffer elements
func NewLRUCache[K comparable, V any](size, buffer int, onEvict func(map[K]V)) *evictionCache[K, V] {
	m := map[K]V{}
	return &evictionCache[K, V]{
		m:              m,
		l:              &sync.RWMutex{},
		evictionPolicy: NewLRUEviction(size, buffer, func(keys []K) { evict(keys, m, onEvict) }),
	}
}

func NewLRUEviction[K comparable](size, buffer int, evict func([]K)) *lruEviction[K] {
	return &lruEviction[K]{
		size:   size,
		cap:    size + buffer,
		keys:   make([]K, 0, size+buffer),
		keySet: make(map[K]struct{}, size+buffer),
		evict:  evict,
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
	// keySet is for faster lookup whether a key exists
	keySet map[K]struct{}
	// evict is called when we evict
	evict func([]K)
	mu    sync.Mutex
}

func (c *lruEviction[K]) Push(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.keySet[key]; ok {
		return
	}
	c.keySet[key] = struct{}{}
	c.keys = append(c.keys, key)
	if len(c.keys) <= c.cap {
		return
	}
	logger.Debugf("Capacity of %d exceeded. Evicting old keys by shifting LRU keys keeping only the %d most recent ones", c.cap, c.size)
	evicted := c.keys[0 : c.cap-c.size+1]
	for _, k := range evicted {
		delete(c.keySet, k)
	}
	c.evict(evicted)
	c.keys = (c.keys)[c.cap-c.size+1:]
}

func (c *lruEviction[K]) String() string {
	return fmt.Sprintf("Keys: [%v], KeySet: [%v]", c.keys, collections.Keys(c.keySet))
}
