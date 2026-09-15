/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lazy

import "sync"

// Provider is a cache of values keyed by input, each produced lazily and at
// most once per key. Unlike [Getter], individual entries can be recomputed:
// Update reruns the provider for one key and Delete evicts it, so a later
// Get produces it again.
type Provider[I any, V any] interface {
	// Get returns the cached value for input, producing and caching it first
	// if this is the first request for that key.
	Get(I) (V, error)
	// Peek returns the cached value for input without producing it, and
	// reports whether one was cached.
	Peek(input I) (V, bool)
	// Update reruns the provider for input and replaces the cached entry,
	// returning the old value (the zero value if there was none) and the new
	// one.
	Update(I) (V, V, error)
	// Delete evicts the cached value for input, if any, and returns it along
	// with whether it was present.
	Delete(I) (V, bool)
	// Length reports the number of cached entries.
	Length() int
}

// NewProvider returns a [Provider] keyed directly by its input.
func NewProvider[K comparable, V any](provider func(K) (V, error)) *lazyProvider[K, K, V] {
	return NewProviderWithKeyMapper[K, K, V](func(k K) K { return k }, provider)
}

// NewProviderWithKeyMapper returns a [Provider] that derives its cache key
// from each input via keyMapper, for inputs that are not themselves
// comparable or that should share a cache entry under some derived identity.
func NewProviderWithKeyMapper[I any, K comparable, V any](keyMapper func(I) K, provider func(I) (V, error)) *lazyProvider[I, K, V] {
	return &lazyProvider[I, K, V]{
		cache:     make(map[K]V),
		provider:  provider,
		keyMapper: keyMapper,
	}
}

type lazyProvider[I any, K comparable, V any] struct {
	cache     map[K]V
	cacheLock sync.RWMutex
	keyMapper func(I) K
	provider  func(I) (V, error)
}

func (v *lazyProvider[I, K, V]) Update(input I) (old, updated V, err error) {
	key := v.keyMapper(input)

	v.cacheLock.Lock()
	defer v.cacheLock.Unlock()
	oldRes := v.cache[key]

	// create the service for the new public params
	res, err := v.provider(input)
	if err != nil {
		var zero V
		return zero, zero, err
	}

	// register the new service
	v.cache[key] = res

	return oldRes, res, nil
}

func (v *lazyProvider[I, K, V]) Get(input I) (V, error) {
	key := v.keyMapper(input)
	if res, ok := v.peekValue(key); ok {
		return res, nil
	}

	// lock
	v.cacheLock.Lock()
	defer v.cacheLock.Unlock()

	// check cache again
	if res, ok := v.cache[key]; ok {
		return res, nil
	}

	// update cache
	res, err := v.provider(input)
	if err != nil {
		var zero V
		return zero, err
	}
	v.cache[key] = res

	return res, nil
}

func (v *lazyProvider[I, K, V]) Peek(input I) (V, bool) {
	return v.peekValue(v.keyMapper(input))
}

func (v *lazyProvider[I, K, V]) peekValue(key K) (V, bool) {
	// Check cache
	v.cacheLock.RLock()
	defer v.cacheLock.RUnlock()
	res, ok := v.cache[key]
	return res, ok
}

func (v *lazyProvider[I, K, V]) Delete(input I) (V, bool) {
	key := v.keyMapper(input)

	v.cacheLock.RLock()
	res, ok := v.cache[key]
	v.cacheLock.RUnlock()

	if ok {
		v.cacheLock.Lock()
		delete(v.cache, key)
		v.cacheLock.Unlock()
	}

	return res, ok
}

func (v *lazyProvider[I, K, V]) Length() int {
	v.cacheLock.RLock()
	defer v.cacheLock.RUnlock()
	return len(v.cache)
}
