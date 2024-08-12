/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lazy

import (
	"io"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

type Holder[V any] interface {
	Get() (V, error)
	Reset() error
}

func NewHolder[V any](provider func() (V, error), closer func(V) error) *lazyHolder[V] {
	return &lazyHolder[V]{provider: provider, closer: closer}
}

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
		h.mu.RUnlock()
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
		return utils.Zero[V](), err
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
	h.v = utils.Zero[V]()
	h.set = false
	return err
}
