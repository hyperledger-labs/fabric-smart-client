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
	"context"
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

	// loadedCh is closed when a value is accepted, releasing WaitForValue
	// callers. It is created lazily by waitChan and replaced by Reset, so a
	// holder returned to its empty state makes later waiters block again
	// rather than being released by an update that has been discarded.
	loadedCh chan struct{}

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
	// Release WaitForValue callers. The write lock is held here, so no waiter
	// can be registering against this channel concurrently.
	if h.loadedCh != nil {
		close(h.loadedCh)
		h.loadedCh = nil
	}
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
// Any WaitForValue caller parked on the previous channel is released. It will
// find the holder unloaded and report ErrNotLoaded, not hang waiting for an
// Update that would close a different channel.
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
	// Release anyone parked on the current channel before dropping it. A
	// waiter released into a still-unloaded holder reports the holder's real
	// state, which is what Get would tell it; leaving the channel unclosed
	// would instead orphan that waiter, because the next Update closes a
	// different channel.
	if h.loadedCh != nil {
		close(h.loadedCh)
		h.loadedCh = nil
	}
}

// waitChan returns the channel that is closed when a value is accepted,
// creating it if this is the first caller. It is separate from WaitForValue so
// that the lock is not held while waiting.
func (h *Holder[T]) waitChan() chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.loadedCh == nil {
		h.loadedCh = make(chan struct{})
	}
	return h.loadedCh
}

// WaitForValue returns the held value, waiting until one is held or ctx is
// done.
//
// It is the blocking counterpart to Get, for a caller that cannot answer
// without the value and would otherwise have to poll: the value is pushed in
// from outside after the owning service is built, so a caller racing that
// arrival is in a normal startup state rather than an error. Prefer Get
// wherever the absent case is something the caller can report and move on
// from.
//
// A refused update is not waited on. Retrying cannot clear one until a later
// update is accepted, so ErrRejected is returned immediately rather than
// holding the caller until its deadline expires. If ctx is done first, its
// error is returned wrapped with the holder's subject, so the caller can tell
// a wait that timed out from one that was cancelled.
func (h *Holder[T]) WaitForValue(ctx context.Context) (T, error) {
	var zero T

	// Fast path, and the rejection check: a holder that already has a value
	// answers without allocating a channel or touching ctx.
	h.mu.RLock()
	loaded, rejected, value := h.loaded, h.rejected, h.value
	h.mu.RUnlock()
	if loaded {
		return value, nil
	}
	if rejected != nil {
		return zero, errors.Wrapf(ErrRejected, "%s rejected: %s", h.subject, rejected)
	}

	ch := h.waitChan()

	// Re-check after taking the channel. An Update or Reset between the read
	// above and waitChan would have closed the previous channel, and this caller
	// would otherwise wait on a channel nothing closes again. After a Reset, the
	// holder is unloaded, so Get will report ErrNotLoaded when we wake.
	h.mu.RLock()
	loaded, rejected, value = h.loaded, h.rejected, h.value
	h.mu.RUnlock()
	if loaded {
		return value, nil
	}
	if rejected != nil {
		return zero, errors.Wrapf(ErrRejected, "%s rejected: %s", h.subject, rejected)
	}

	select {
	case <-ch:
		v, err := h.Get()
		if err != nil {
			return zero, err
		}
		return v, nil
	case <-ctx.Done():
		return zero, errors.Wrapf(ctx.Err(), "timed out waiting for %s", h.subject)
	}
}
