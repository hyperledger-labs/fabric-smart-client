/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	"sync"
	"time"
)

// NewTimeoutCache creates a cache that keeps elements for evictionTimeout time.
// An element might return even if it is marked stale.
func NewTimeoutCache[K comparable, V any](evictionTimeout time.Duration, onEvict func(map[K]V)) *evictionCache[K, V] {
	m := map[K]V{}
	l := &sync.RWMutex{}
	return &evictionCache[K, V]{
		m: m,
		l: l,
		evictionPolicy: NewTimeoutEviction(evictionTimeout, func(keys []K) {
			logger.Debugf("Evicting stale keys: [%v]", keys)
			l.Lock()
			defer l.Unlock()
			evict(keys, m, onEvict)
		}),
	}
}

type timeoutEviction[K comparable] struct {
	keys  []timeoutEntry[K]
	mu    sync.RWMutex
	evict func([]K)
}

type timeoutEntry[K comparable] struct {
	created time.Time
	key     K
}

func NewTimeoutEviction[K comparable](timeout time.Duration, evict func([]K)) *timeoutEviction[K] {
	e := &timeoutEviction[K]{
		keys:  make([]timeoutEntry[K], 0),
		evict: evict,
	}
	go e.cleanup(timeout)
	return e
}

func (e *timeoutEviction[K]) cleanup(timeout time.Duration) {
	logger.Debugf("Launch cleanup function with eviction timeout [%v]", timeout)

	// TODO: the cleanup should be revisited
	// when the cleanup is started we loop forever using the ticker; even if there are no items in the cache anymore.
	// this might also prevent the GC from removing the cache if not needed anymore

	// let's use the eviction timeout as our check interval
	checkInterval := timeout

	for range time.Tick(checkInterval) {
		expiry := time.Now().Add(-timeout)
		logger.Debugf("Cleanup invoked: evicting everything created after [%v]", expiry)
		e.mu.RLock()
		evicted := make([]K, 0)
		for _, entry := range e.keys {
			if entry.created.After(expiry) {
				break
			}
			evicted = append(evicted, entry.key)
		}
		e.mu.RUnlock()
		if len(evicted) > 0 {
			e.mu.Lock()
			e.keys = e.keys[len(evicted):]
			e.mu.Unlock()
			logger.Debugf("Evicting %d entries", len(evicted))
			e.evict(evicted)
		}
	}
}

func (e *timeoutEviction[K]) Push(key K) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.keys = append(e.keys, timeoutEntry[K]{key: key, created: time.Now()})
}
