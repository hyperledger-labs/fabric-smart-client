/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lazy

import (
	"io"
	"sync"
)

// Holder computes its value on first use, from the provider it was given, and
// keeps it until Reset.
//
// The push-shaped counterpart is deferred.Holder, which is handed its value from
// outside instead of producing it. Reach for that one when the value arrives
// later — a configuration block, a context handed over at startup — and for this
// one when the owner can produce it itself.
type Holder[V any] interface {
	Get() (V, error)
	Reset() error
}

// NewHolder returns a [Holder] that produces its value from provider on
// first use and, on Reset, releases it through closer.
func NewHolder[V any](provider func() (V, error), closer func(V) error) *lazyHolder[V] {
	return &lazyHolder[V]{provider: provider, closer: closer}
}

// NewCloserHolder is [NewHolder] for a value that releases itself, using its
// own Close method as the closer.
func NewCloserHolder[V io.Closer](provider func() (V, error)) *lazyHolder[V] {
	return &lazyHolder[V]{provider: provider, closer: func(v V) error { return v.Close() }}
}

type lazyHolder[V any] struct {
	v        V
	provider func() (V, error)
	closer   func(V) error
	mu       sync.RWMutex
	set      bool
}

func (h *lazyHolder[V]) Get() (V, error) {
	h.mu.RLock()
	if h.set {
		defer h.mu.RUnlock()
		return h.v, nil
	}
	h.mu.RUnlock()

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.set {
		return h.v, nil
	}

	v, err := h.provider()
	if err != nil {
		var zero V
		return zero, err
	}

	h.v = v
	h.set = true

	return v, nil
}

func (h *lazyHolder[V]) Reset() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var err error
	if h.set {
		err = h.closer(h.v)
	}
	var zero V
	h.v = zero
	h.set = false
	return err
}
