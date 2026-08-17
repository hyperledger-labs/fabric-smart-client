/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	"context"
	"sync"
	"time"
)

// NewTimeoutCache creates a cache that keeps elements for evictionTimeout time.
// An element might return even if it is marked stale.
// The background cleanup goroutine stops when ctx is cancelled.
// A non-positive evictionTimeout disables eviction; see NewTimeoutEviction.
func NewTimeoutCache[K comparable, V any](ctx context.Context, evictionTimeout time.Duration, onEvict func(map[K]V)) *evictionCache[K, V] {
	m := map[K]V{}
	l := &sync.RWMutex{}
	return &evictionCache[K, V]{
		m: m,
		l: l,
		evictionPolicy: NewTimeoutEviction(ctx, evictionTimeout, func(keys []K) {
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

// NewTimeoutEviction returns an eviction policy that evicts entries older than timeout,
// driven by a background goroutine that stops when ctx is cancelled.
//
// A non-positive timeout disables eviction entirely and no goroutine is started. Note
// that time.NewTicker panics on a non-positive duration, so this guard is what keeps a
// misconfigured timeout from taking the process down from inside the goroutine.
func NewTimeoutEviction[K comparable](ctx context.Context, timeout time.Duration, evict func([]K)) *timeoutEviction[K] {
	e := &timeoutEviction[K]{
		keys:  make([]timeoutEntry[K], 0),
		evict: evict,
	}
	if timeout <= 0 {
		logger.Warnf("Eviction timeout is [%v]; eviction is disabled and entries are kept until the cache is dropped", timeout)
		return e
	}
	go e.cleanup(ctx, timeout)
	return e
}

func (e *timeoutEviction[K]) cleanup(ctx context.Context, timeout time.Duration) {
	logger.Debugf("Launch cleanup function with eviction timeout [%v]", timeout)

	// let's use the eviction timeout as our check interval
	ticker := time.NewTicker(timeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debugf("Stopping cleanup: context cancelled")
			return
		case <-ticker.C:
		}
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
