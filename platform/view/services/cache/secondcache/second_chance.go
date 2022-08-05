/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package secondcache

import (
	"sync"
	"sync/atomic"
)

// This package implements Second-Chance Algorithm, an approximate LRU algorithms.
// https://www.cs.jhu.edu/~yairamir/cs418/os6/tsld023.htm

// secondChanceCache holds key-value items with a limited size.
// When the number cached items exceeds the limit, victims are selected based on the
// Second-Chance Algorithm and Get purged
type secondChanceCache struct {
	// manages mapping between keys and items
	table map[string]*cacheItem

	// holds a list of cached items.
	items []*cacheItem

	// indicates the next candidate of a victim in the items list
	position int

	// read lock for Get, and write lock for Add
	rwlock sync.RWMutex
}

type cacheItem struct {
	key   string
	value interface{}
	// set to 1 when Get() is called. set to 0 when victim scan
	referenced int32
}

func New(cacheSize int) *secondChanceCache {
	var cache secondChanceCache
	cache.position = 0
	cache.items = make([]*cacheItem, cacheSize)
	cache.table = make(map[string]*cacheItem)

	return &cache
}

func (cache *secondChanceCache) Get(key string) (interface{}, bool) {
	cache.rwlock.RLock()
	defer cache.rwlock.RUnlock()

	item, ok := cache.table[key]
	if !ok {
		return nil, false
	}

	// referenced bit is set to true to indicate that this item is recently accessed.
	atomic.StoreInt32(&item.referenced, 1)

	return item.value, true
}

func (cache *secondChanceCache) Add(key string, value interface{}) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	if old, ok := cache.table[key]; ok {
		old.value = value
		atomic.StoreInt32(&old.referenced, 1)
		return
	}

	var item cacheItem
	item.key = key
	item.value = value

	size := len(cache.items)
	num := len(cache.table)
	if num < size {
		// cache is not full, so just store the new item at the end of the list
		cache.table[key] = &item
		cache.items[num] = &item
		return
	}

	// starts victim scan since cache is full
	for {
		// checks whether this item is recently accessed or not
		victim := cache.items[cache.position]
		if atomic.LoadInt32(&victim.referenced) == 0 {
			// a victim is found. delete it, and store the new item here.
			delete(cache.table, victim.key)
			cache.table[key] = &item
			cache.items[cache.position] = &item
			cache.position = (cache.position + 1) % size
			return
		}

		// referenced bit is set to false so that this item will be Get purged
		// unless it is accessed until a next victim scan
		atomic.StoreInt32(&victim.referenced, 0)
		cache.position = (cache.position + 1) % size
	}
}

func (cache *secondChanceCache) Delete(key string) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	if old, ok := cache.table[key]; ok {
		old.value = nil
		atomic.StoreInt32(&old.referenced, 1)
		return
	}
}

type Slice [64]byte

type secondChanceCacheBytes struct {
	// manages mapping between keys and items
	table map[Slice]*cacheItemBytes

	// holds a list of cached items.
	items []*cacheItemBytes

	// indicates the next candidate of a victim in the items list
	position int

	// read lock for Get, and write lock for Add
	rwlock sync.RWMutex
}

type cacheItemBytes struct {
	key   Slice
	value interface{}
	// set to 1 when Get() is called. set to 0 when victim scan
	referenced int32
}

func NewBytes(cacheSize int) *secondChanceCacheBytes {
	var cache secondChanceCacheBytes
	cache.position = 0
	cache.items = make([]*cacheItemBytes, cacheSize)
	cache.table = make(map[Slice]*cacheItemBytes)

	return &cache
}

func (cache *secondChanceCacheBytes) Get(key []byte) (interface{}, bool) {
	cache.rwlock.RLock()
	defer cache.rwlock.RUnlock()

	item, ok := cache.table[cache.key(key)]
	if !ok {
		return nil, false
	}

	// referenced bit is set to true to indicate that this item is recently accessed.
	atomic.StoreInt32(&item.referenced, 1)

	return item.value, true
}

func (cache *secondChanceCacheBytes) Add(key []byte, value interface{}) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	k := cache.key(key)
	if old, ok := cache.table[k]; ok {
		old.value = value
		atomic.StoreInt32(&old.referenced, 1)
		return
	}

	var item cacheItemBytes
	item.key = k
	item.value = value

	size := len(cache.items)
	num := len(cache.table)
	if num < size {
		// cache is not full, so just store the new item at the end of the list
		cache.table[k] = &item
		cache.items[num] = &item
		return
	}

	// starts victim scan since cache is full
	for {
		// checks whether this item is recently accessed or not
		victim := cache.items[cache.position]
		if atomic.LoadInt32(&victim.referenced) == 0 {
			// a victim is found. delete it, and store the new item here.
			delete(cache.table, victim.key)
			cache.table[k] = &item
			cache.items[cache.position] = &item
			cache.position = (cache.position + 1) % size
			return
		}

		// referenced bit is set to false so that this item will be Get purged
		// unless it is accessed until a next victim scan
		atomic.StoreInt32(&victim.referenced, 0)
		cache.position = (cache.position + 1) % size
	}
}

func (cache *secondChanceCacheBytes) Delete(key []byte) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	if old, ok := cache.table[cache.key(key)]; ok {
		old.value = nil
		atomic.StoreInt32(&old.referenced, 1)
		return
	}
}

func (cache *secondChanceCacheBytes) key(k []byte) Slice {
	// hash k using sha256
	var key Slice
	copy(key[:], k)
	return key
}
