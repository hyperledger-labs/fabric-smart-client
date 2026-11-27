/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lazy

import "sync"

// Provider is a cache of values keyed by input, each produced lazily and at
// most once per key. Individual entries can be recomputed:
// Update reruns the provider for one key and Delete evicts it, so a later
// Get produces it again.
//
// Builds of one key (a Get of an uncached key, or an Update) and its Delete
// run one at a time, so a build never overlaps another build or the eviction
// of its key. Builds of
// different keys run concurrently, so the provider function must be safe for
// concurrent use with different keys. It must not call Get, Update or Delete
// of its own provider: for the key it builds that deadlocks at once, and for
// other keys two builds can wait for each other. Peek, Length and a Get of a
// cached key never wait for a build.
type Provider[I any, V any] interface {
	// Get returns the cached value for input, producing and caching it first
	// if this is the first request for that key. A failed build caches
	// nothing, so the next Get for that key builds again.
	Get(I) (V, error)
	// Peek returns the cached value for input without producing it, and
	// reports whether one was cached.
	Peek(input I) (V, bool)
	// Update reruns the provider for input and replaces the cached entry,
	// returning the old value (the zero value if there was none) and the new
	// one. Until it returns, Get and Peek return the old value. If the build
	// fails, the cached entry is left as it was.
	Update(I) (V, V, error)
	// Delete evicts the cached value for input, if any, and returns it along
	// with whether it was present. It waits for a running build of that key,
	// so it evicts the value that build produces.
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
		keyLocks:  make(map[K]*keyLock),
		provider:  provider,
		keyMapper: keyMapper,
	}
}

type lazyProvider[I any, K comparable, V any] struct {
	// cacheLock guards cache and keyLocks. It is held only for map access,
	// never while the provider function runs.
	cacheLock sync.RWMutex
	cache     map[K]V
	keyLocks  map[K]*keyLock
	keyMapper func(I) K
	provider  func(I) (V, error)
}

// keyLock serializes Get, Update and Delete of one key. users counts the
// goroutines holding or waiting for mu, so the last one can remove it.
type keyLock struct {
	mu    sync.Mutex
	users int
}

// lockKey locks key and returns the function that unlocks it.
func (v *lazyProvider[I, K, V]) lockKey(key K) (unlock func()) {
	v.cacheLock.Lock()
	l, ok := v.keyLocks[key]
	if !ok {
		l = &keyLock{}
		v.keyLocks[key] = l
	}
	l.users++
	v.cacheLock.Unlock()

	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		v.cacheLock.Lock()
		l.users--
		if l.users == 0 {
			delete(v.keyLocks, key)
		}
		v.cacheLock.Unlock()
	}
}

func (v *lazyProvider[I, K, V]) Update(input I) (old, updated V, err error) {
	key := v.keyMapper(input)
	defer v.lockKey(key)()

	res, err := v.provider(input)
	if err != nil {
		var zero V
		return zero, zero, err
	}

	v.cacheLock.Lock()
	defer v.cacheLock.Unlock()
	old = v.cache[key]
	v.cache[key] = res
	return old, res, nil
}

func (v *lazyProvider[I, K, V]) Get(input I) (V, error) {
	key := v.keyMapper(input)
	if res, ok := v.peekValue(key); ok {
		return res, nil
	}

	defer v.lockKey(key)()

	// another Get may have built it while this one waited for the key
	if res, ok := v.peekValue(key); ok {
		return res, nil
	}

	res, err := v.provider(input)
	if err != nil {
		var zero V
		return zero, err
	}

	v.cacheLock.Lock()
	defer v.cacheLock.Unlock()
	v.cache[key] = res
	return res, nil
}

func (v *lazyProvider[I, K, V]) Peek(input I) (V, bool) {
	return v.peekValue(v.keyMapper(input))
}

func (v *lazyProvider[I, K, V]) peekValue(key K) (V, bool) {
	v.cacheLock.RLock()
	defer v.cacheLock.RUnlock()
	res, ok := v.cache[key]
	return res, ok
}

func (v *lazyProvider[I, K, V]) Delete(input I) (V, bool) {
	key := v.keyMapper(input)
	defer v.lockKey(key)()

	v.cacheLock.Lock()
	defer v.cacheLock.Unlock()
	res, ok := v.cache[key]
	delete(v.cache, key)
	return res, ok
}

func (v *lazyProvider[I, K, V]) Length() int {
	v.cacheLock.RLock()
	defer v.cacheLock.RUnlock()
	return len(v.cache)
}
