/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package configstate holds channel configuration that is populated
// asynchronously, after the service owning it has been constructed.
package configstate

import (
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// Holder guards a channel configuration of type T against concurrent access and
// against being read before it exists.
//
// Channel configuration is not available when a membership service is built: it
// arrives later, when the first configuration block is committed or the config
// monitor completes its first poll. Until then the holder is empty, which is a
// normal startup state rather than an error in the caller.
//
// Get is the accessor to reach for. Because it returns an error alongside the
// configuration rather than the configuration alone, the absent case is put in
// front of the caller at the point of use instead of surfacing as a nil
// dereference somewhere further down. Discarding the error still compiles, so
// this makes the mistake visible rather than impossible.
type Holder[T any] struct {
	// mu serializes access to value and loaded.
	mu sync.RWMutex
	// value is the configuration held, meaningful only when loaded is true.
	value T
	// loaded records whether value has been set. It is tracked explicitly
	// rather than by comparing value against nil so that T may be an
	// interface type whose stored value is legitimately nil.
	loaded bool

	channelName string
}

// NewHolder returns an empty Holder for the named channel. The channel name is
// only used to give the errors returned by Get somewhere to point; the service
// owning the holder remains the authority on channel identity.
func NewHolder[T any](channelName string) *Holder[T] {
	return &Holder[T]{channelName: channelName}
}

// Get returns the held configuration. Until the first successful Update it
// returns an error wrapping driver.ErrNotInitialized, which callers that can
// tolerate the startup race may test for with errors.Is.
func (h *Holder[T]) Get() (T, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.loaded {
		var zero T
		return zero, errors.Wrapf(driver.ErrNotInitialized, "channel [%s] configuration not loaded", h.channelName)
	}

	return h.value, nil
}

// TryGet returns the held configuration and whether one is held. Prefer Get;
// use TryGet only where absence is a legitimate outcome to be handled inline
// rather than reported: satisfying an interface that expresses "no
// configuration" as a nil return, or working from a snapshot of whatever is
// held at the time of the call.
func (h *Holder[T]) TryGet() (T, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.value, h.loaded
}

// Update replaces the held configuration with the value produced by fn, which
// receives the currently held value along with whether one is held. The held
// value is left untouched if fn returns an error, so a rejected configuration
// cannot leave the holder empty or stale.
//
// fn runs while the write lock is held and must not call back into this Holder;
// doing so deadlocks.
func (h *Holder[T]) Update(fn func(current T, loaded bool) (T, error)) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	v, err := fn(h.value, h.loaded)
	if err != nil {
		return err
	}

	h.value = v
	h.loaded = true
	return nil
}
