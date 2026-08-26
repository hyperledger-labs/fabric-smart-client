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
//
// An empty holder distinguishes two ways of being empty. One has not been
// offered a configuration yet, and a caller racing that arrival should retry;
// the other was offered one and refused it, and retrying will not help until a
// later update is accepted. Get names them with driver.ErrNotInitialized and
// driver.ErrConfigRejected respectively.
type Holder[T any] struct {
	// mu serializes access to value, loaded and rejected.
	mu sync.RWMutex
	// value is the configuration held, meaningful only when loaded is true.
	value T
	// loaded records whether value has been set. It is tracked explicitly
	// rather than by comparing value against nil so that T may be an
	// interface type whose stored value is legitimately nil.
	loaded bool
	// rejected is the error from the most recent refused Update, kept only
	// while no configuration has ever been accepted. Once loaded is true the
	// holder can answer from value and a refusal is the updater's problem
	// rather than the reader's, so this is cleared and not consulted.
	rejected error

	// subject names what is held, as a noun phrase that reads correctly in
	// front of "not loaded" — "channel [mychannel] configuration", say. It is
	// only used to give the errors returned by Get somewhere to point; the
	// service owning the holder remains the authority on identity.
	subject string
}

// NewHolder returns an empty Holder for the named subject. See the subject
// field for how the name is used.
func NewHolder[T any](subject string) *Holder[T] {
	return &Holder[T]{subject: subject}
}

// Get returns the held configuration.
//
// Before the first successful Update it returns an error instead, naming which
// kind of empty the holder is in so that callers can tell a startup race from a
// configuration that was refused:
//
//   - driver.ErrNotInitialized, if no update has been attempted. The
//     configuration is still on its way and a caller that can tolerate the race
//     may test for this with errors.Is and retry.
//   - driver.ErrConfigRejected, if every update so far was refused. The error
//     that refused the most recent one is wrapped, and retrying will not clear
//     it until a later update is accepted.
//
// Once an update has been accepted Get answers from the held value and returns
// no error, including when a later update is refused; see Update.
func (h *Holder[T]) Get() (T, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.loaded {
		var zero T
		if h.rejected != nil {
			return zero, errors.Wrapf(driver.ErrConfigRejected, "%s rejected: %s", h.subject, h.rejected)
		}
		return zero, errors.Wrapf(driver.ErrNotInitialized, "%s not loaded", h.subject)
	}

	return h.value, nil
}

// TryGet returns the held configuration and whether one is held. Prefer Get;
// use TryGet only where absence is a legitimate outcome to be handled inline
// rather than reported: satisfying an interface that expresses "no
// configuration" as a nil return, or working from a snapshot of whatever is
// held at the time of the call.
//
// TryGet reports only whether a configuration is held, and a refused update
// never becomes one. Callers that need to tell a refusal apart from a startup
// race must use Get.
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
// What a refusal means to readers depends on whether anything has been accepted
// yet. With a configuration already in force the holder keeps answering from it
// and readers see nothing; the refusal is reported to the updater alone, through
// the error returned here. With nothing in force the holder remembers the
// refusal, so that Get can report driver.ErrConfigRejected rather than inviting
// a retry that cannot help. Either way an accepted update clears it.
//
// fn runs while the write lock is held and must not call back into this Holder;
// doing so deadlocks.
func (h *Holder[T]) Update(fn func(current T, loaded bool) (T, error)) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	v, err := fn(h.value, h.loaded)
	if err != nil {
		if !h.loaded {
			h.rejected = err
		}
		return err
	}

	h.value = v
	h.loaded = true
	h.rejected = nil
	return nil
}

// Reset returns the holder to the state it was constructed in: nothing held,
// and no record of a refusal.
//
// For an owner that rebuilds what it holds from scratch rather than replacing it
// in one step — clearing its caches, reloading, and installing whatever the
// reload produces. Without this, a reload that fails to produce anything would
// leave the previous value in place and Get would keep answering with it, and
// because a refusal is only recorded while nothing is held, the reason the
// reload produced nothing would be discarded too.
//
// Callers that can replace the value in a single Update should do that instead:
// this opens a window in which Get reports driver.ErrNotInitialized.
func (h *Holder[T]) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	var zero T
	h.value = zero
	h.loaded = false
	h.rejected = nil
}
