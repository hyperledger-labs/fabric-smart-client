/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

// NewMapCache creates a dummy implementation of the Cache interface.
// It is backed by a map with unlimited capacity.
func NewMapCache[K comparable, V any]() *mapCache[K, V] {
	return &mapCache[K, V]{
		m: map[K]V{},
		l: &noLock{},
	}
}

type mapCache[K comparable, V any] struct {
	m map[K]V
	l rwLock
}

//nolint:revive // confusing-naming: mapCache and evictionCache both implement the exported Map interface; renaming either is an API break; see follow-up
func (c *mapCache[K, V]) Get(key K) (V, bool) {
	c.l.RLock()
	defer c.l.RUnlock()
	v, ok := c.m[key]
	return v, ok
}

//nolint:revive // confusing-naming: mapCache and evictionCache both implement the exported Map interface; renaming either is an API break; see follow-up
func (c *mapCache[K, V]) Put(key K, value V) {
	c.l.Lock()
	defer c.l.Unlock()
	c.m[key] = value
}

//nolint:revive // confusing-naming: mapCache and evictionCache both implement the exported Map interface; renaming either is an API break; see follow-up
func (c *mapCache[K, V]) Delete(keys ...K) {
	c.l.Lock()
	defer c.l.Unlock()
	for _, key := range keys {
		delete(c.m, key)
	}
}

//nolint:revive // confusing-naming: mapCache and evictionCache both implement the exported Map interface; renaming either is an API break; see follow-up
func (c *mapCache[K, V]) Update(key K, f func(bool, V) (bool, V)) bool {
	c.l.Lock()
	defer c.l.Unlock()
	v, ok := c.m[key]
	keep, newValue := f(ok, v)
	if !keep {
		delete(c.m, key)
	} else {
		c.m[key] = newValue
	}
	return ok
}

//nolint:revive // confusing-naming: mapCache and evictionCache both implement the exported Map interface; renaming either is an API break; see follow-up
func (c *mapCache[K, V]) Len() int {
	return len(c.m)
}
