/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package lazy provides value holders that defer producing their value until
// it is first requested, and memoize the result (including an error) for
// every later request.
package lazy

import "sync"

// Getter produces and caches a single value on first use.
//
// Unlike [Holder], a Getter cannot be reset: once its provider has run, every
// later Get returns the same value or error, forever.
type Getter[V any] interface {
	Get() (V, error)
}

type lazyGetter[V any] struct {
	get func() (V, error)
}

// NewGetter returns a [Getter] that calls provider at most once, the first
// time Get is called, and caches whatever it returns, error included. It is
// safe for concurrent use.
func NewGetter[V any](provider func() (V, error)) *lazyGetter[V] {
	return &lazyGetter[V]{get: sync.OnceValues(provider)}
}

// Get returns the memoized value, running the provider first if this is the
// first call.
func (g *lazyGetter[V]) Get() (V, error) {
	return g.get()
}
