/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deferred

// Test support for holder_test.go, which is package deferred_test and so can
// neither declare a method on Holder nor read the unexported state below. That
// is why this lives in its own file rather than beside the tests that use it.

// HasWaiter reports whether a [Holder.WaitForValue] caller has registered for
// the holder's next answer.
//
// It exists so that a test can order an Update or Reset after a waiter has
// registered without approximating that with a sleep, which is what makes a
// concurrency test flake on a loaded machine. Registering is the point that
// matters: a waiter that has taken the channel is released by a close on it
// whether or not it has reached its select yet.
func (h *Holder[T]) HasWaiter() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.loadedCh != nil
}
