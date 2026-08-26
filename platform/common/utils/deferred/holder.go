/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package deferred holds a value that is supplied after the service owning it
// has been constructed, and keeps readers from seeing it before it exists.
//
// The pull-shaped counterpart is [lazy.Holder], which computes its value on
// demand from a provider. Reach for this one when the value is pushed in from
// outside — a configuration block that arrives later, a context handed over at
// startup — and for lazy.Holder when the owner can produce it itself.
package deferred

import (
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// ErrNotLoaded signals that a holder cannot answer yet because nothing has been
// supplied to it.
//
// This is a transient startup condition, not a permanent failure: the value
// arrives after the owning service is constructed, so a caller racing that
// arrival observes it legitimately and may test for it with errors.Is and retry.
var ErrNotLoaded = errors.New("not initialized")

// ErrRejected signals that a holder cannot answer because the only value it has
// been offered was refused by the owner's own update function.
//
// Unlike ErrNotLoaded this is not a startup race, and retrying will not clear
// it. It is not permanent either: the holder recovers as soon as a later update
// is accepted, so a caller should surface it rather than either retrying in a
// loop or treating the owner as dead. The error that refused the update is
// wrapped, so the reason travels with the sentinel.
var ErrRejected = errors.New("configuration rejected")

// Holder guards a value of type T against concurrent access and against being
// read before it exists.
//
// The value is not available when the owning service is built: it arrives later,
// pushed in through Update. Until then the holder is empty, which is a normal
// startup state rather than an error in the caller.
//
// Get is the accessor to reach for. Because it returns an error alongside the
// value rather than the value alone, the absent case is put in front of the
// caller at the point of use instead of surfacing as a nil dereference somewhere
// further down. Discarding the error still compiles, so this makes the mistake
// visible rather than impossible.
//
// An empty holder distinguishes two ways of being empty. One has not been
// offered a value yet, and a caller racing that arrival should retry; the other
// was offered one and refused it, and retrying will not help until a later
// update is accepted. Get names them with ErrNotLoaded and ErrRejected
// respectively.
type Holder[T any] struct {
	// mu serializes access to value, loaded and rejected.
	mu sync.RWMutex
	// value is what is held, meaningful only when loaded is true.
	value T
	// loaded records whether value has been set. It is tracked explicitly
	// rather than by comparing value against nil so that T may be an
	// interface type whose stored value is legitimately nil.
	loaded bool
	// rejected is the error from the most recent refused Update, kept only
	// while no value has ever been accepted. Once loaded is true the
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

// Get returns the held value.
//
// Before the first successful Update it returns an error instead, naming which
// kind of empty the holder is in so that callers can tell a startup race from a
// value that was refused:
//
//   - ErrNotLoaded, if no update has been attempted. The value is still on its
//     way and a caller that can tolerate the race may test for this with
//     errors.Is and retry.
//   - ErrRejected, if every update so far was refused. The error that refused
//     the most recent one is wrapped, and retrying will not clear it until a
//     later update is accepted.
//
// Once an update has been accepted Get answers from the held value and returns
// no error, including when a later update is refused; see Update.
func (h *Holder[T]) Get() (T, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.loaded {
		var zero T
		if h.rejected != nil {
			return zero, errors.Wrapf(ErrRejected, "%s rejected: %s", h.subject, h.rejected)
		}
		return zero, errors.Wrapf(ErrNotLoaded, "%s not loaded", h.subject)
	}

	return h.value, nil
}

// TryGet returns the held value and whether one is held. Prefer Get; use TryGet
// only where absence is a legitimate outcome to be handled inline rather than
// reported: satisfying an interface that expresses "nothing here" as a nil
// return, or working from a snapshot of whatever is held at the time of the
// call.
//
// TryGet reports only whether a value is held, and a refused update never
// becomes one. Callers that need to tell a refusal apart from a startup
// race must use Get.
func (h *Holder[T]) TryGet() (T, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.value, h.loaded
}

// Update replaces the held value with the one produced by fn, which receives the
// currently held value along with whether one is held. The held value is left
// untouched if fn returns an error, so a rejected update cannot leave the holder
// empty or stale.
//
// What a refusal means to readers depends on whether anything has been accepted
// yet. With a value already in force the holder keeps answering from it and
// readers see nothing; the refusal is reported to the updater alone, through the
// error returned here. With nothing in force the holder remembers the refusal,
// so that Get can report ErrRejected rather than inviting a retry that cannot
// help. Either way an accepted update clears it.
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
// this opens a window in which Get reports ErrNotLoaded.
func (h *Holder[T]) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	var zero T
	h.value = zero
	h.loaded = false
	h.rejected = nil
}
