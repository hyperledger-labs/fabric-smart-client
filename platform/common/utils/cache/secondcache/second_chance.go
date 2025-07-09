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

// typedSecondChanceCache holds key-value items with a limited size.
// When the number cached items exceeds the limit, victims are selected based on the
// Second-Chance Algorithm and Get purged
type typedSecondChanceCache[T any] struct {
	// manages mapping between keys and items
	table map[string]*cacheItem[T]

	// holds a list of cached items.
	items []*cacheItem[T]

	buffer int

	// indicates the next candidate of a victim in the items list
	position int

	// read lock for Get, and write lock for Add
	rwlock sync.RWMutex
}

type cacheItem[T any] struct {
	key   string
	value T
	// set to 1 when Get() is called. set to 0 when victim scan
	referenced int32
}

type secondChanceCache = typedSecondChanceCache[interface{}]

func New(cacheSize int) *secondChanceCache {
	return NewTyped[interface{}](cacheSize)
}

func NewTyped[T any](cacheSize int) *typedSecondChanceCache[T] {
	var cache typedSecondChanceCache[T]
	cache.position = 0
	cache.buffer = max(cacheSize/3, 1) // One third of the entries will be evicted when the cache is full
	cache.items = make([]*cacheItem[T], cacheSize)
	cache.table = make(map[string]*cacheItem[T], cacheSize)

	return &cache
}

func (cache *typedSecondChanceCache[T]) Get(key string) (T, bool) {
	cache.rwlock.RLock()
	defer cache.rwlock.RUnlock()

	return cache.get(key)
}

func (cache *typedSecondChanceCache[T]) get(key string) (T, bool) {
	item, ok := cache.table[key]
	if !ok {
		return zero[T](), false
	}

	// referenced bit is set to true to indicate that this item is recently accessed.
	atomic.StoreInt32(&item.referenced, 1)

	return item.value, true
}

func (cache *typedSecondChanceCache[T]) GetOrLoad(key string, loader func() (T, error)) (T, bool, error) {
	cache.rwlock.RLock()

	if value, ok := cache.get(key); ok {
		cache.rwlock.RUnlock()
		return value, true, nil
	}
	cache.rwlock.RUnlock()

	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	if value, ok := cache.get(key); ok {
		return value, true, nil
	}

	value, err := loader()
	if err != nil {
		return zero[T](), false, err
	}

	cache.add(key, value)

	return value, false, nil
}

func (cache *typedSecondChanceCache[T]) Add(key string, value T) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	cache.add(key, value)
}

func (cache *typedSecondChanceCache[T]) add(key string, value T) {
	if old, ok := cache.table[key]; ok {
		old.value = value
		atomic.StoreInt32(&old.referenced, 1)
		return
	}

	var item cacheItem[T]
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
	for evicted := 0; evicted < cache.buffer; {
		// checks whether this item is recently accessed or not
		victim := cache.items[cache.position]
		if atomic.LoadInt32(&victim.referenced) == 0 {
			// a victim is found. delete it, and store the new item here.
			delete(cache.table, victim.key)
			cache.table[key] = &item
			cache.items[cache.position] = &item
			cache.position = (cache.position + 1) % size
			evicted++
			continue
		}

		// referenced bit is set to false so that this item will be Get purged
		// unless it is accessed until a next victim scan
		atomic.StoreInt32(&victim.referenced, 0)
		cache.position = (cache.position + 1) % size
	}
}

func (cache *typedSecondChanceCache[T]) Delete(key string) {
	cache.rwlock.Lock()
	defer cache.rwlock.Unlock()

	if old, ok := cache.table[key]; ok {
		old.value = zero[T]()
		atomic.StoreInt32(&old.referenced, 1)
		return
	}
}

func zero[T any]() T {
	var result T
	return result
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
