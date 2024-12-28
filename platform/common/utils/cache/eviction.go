/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import "fmt"

func evict[K comparable, V any](keys []K, m map[K]V, onEvict func(map[K]V)) {
	evicted := make(map[K]V, len(keys))
	for _, k := range keys {
		if v, ok := m[k]; ok {
			evicted[k] = v
			delete(m, k)
		} else {
			logger.Debugf("No need to evict [%k]. Was already deleted.")
		}
	}
	onEvict(evicted)
}

type evictionCache[K comparable, V any] struct {
	m              map[K]V
	l              rwLock
	evictionPolicy EvictionPolicy[K]
}

func (c *evictionCache[K, V]) String() string {
	return fmt.Sprintf("Content: [%v], Eviction policy: [%v]", c.m, c.evictionPolicy)
}

type EvictionPolicy[K comparable] interface {
	// Push adds a key and must be invoked under write-lock
	Push(K)
}

func (c *evictionCache[K, V]) Get(key K) (V, bool) {
	c.l.RLock()
	defer c.l.RUnlock()
	v, ok := c.m[key]
	return v, ok
}

func (c *evictionCache[K, V]) Put(key K, value V) {
	c.l.Lock()
	defer c.l.Unlock()
	c.m[key] = value
	// We assume that a value is always new for performance reasons.
	// If we try to put again a value, this value will be put also in the LRU keys instead of just promoting the existing one.
	// If we put this value c.cap times, then this will evict all other values.
	c.evictionPolicy.Push(key)
}

func (c *evictionCache[K, V]) Update(key K, f func(bool, V) (bool, V)) bool {
	c.l.Lock()
	defer c.l.Unlock()
	v, ok := c.m[key]
	keep, newValue := f(ok, v)
	if !keep {
		delete(c.m, key)
	} else {
		c.m[key] = newValue
	}
	if !ok && keep {
		c.evictionPolicy.Push(key)
	}
	return ok
}

func (c *evictionCache[K, V]) Delete(keys ...K) {
	c.l.Lock()
	defer c.l.Unlock()
	for _, key := range keys {
		delete(c.m, key)
	}
}

func (c *evictionCache[K, V]) Len() int {
	c.l.RLock()
	defer c.l.RUnlock()
	return len(c.m)
}
