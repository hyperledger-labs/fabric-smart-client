/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators_test

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
)

// errAt is returned by the failing test iterators below, so assertions can match
// on identity rather than on a message.
var errAt = errors.New("iterator failure")

// ptrs turns values into the pointer slice most of the package's helpers expect.
// Single elements use the builtin new(v) instead.
func ptrs[T any](vs ...T) []*T {
	out := make([]*T, 0, len(vs))
	for i := range vs {
		out = append(out, &vs[i])
	}
	return out
}

// failingIterator yields items[:failAt] and then fails every call with errAt,
// without advancing, so a test that keeps reading sees the error again rather
// than skipping the element it failed on. Use newInfallible for a source that
// only ever reaches exhaustion.
//
// calls counts every Next call and closed records Close, so tests can assert a
// helper neither over-reads nor leaks the iterator it consumes.
type failingIterator[T any] struct {
	items  []T
	i      int
	failAt int
	calls  int
	closed bool
}

func newFailing[T any](items []T, failAt int) *failingIterator[T] {
	return &failingIterator[T]{items: items, failAt: failAt}
}

// newInfallible builds a source that yields every item and then exhausts. The
// negative failAt is never reached, since the position only ever grows.
func newInfallible[T any](items []T) *failingIterator[T] {
	return newFailing(items, -1)
}

func (it *failingIterator[T]) Next() (T, error) {
	var zero T
	it.calls++
	if it.i == it.failAt {
		return zero, errAt
	}
	if it.i >= len(it.items) {
		return zero, nil
	}
	item := it.items[it.i]
	it.i++
	return item, nil
}

func (it *failingIterator[T]) Close() { it.closed = true }

// closeRecorder wraps a slice iterator and records Close calls.
type closeRecorder[T any] struct {
	iterators.Iterator[T]
	closed bool
}

func newCloseRecorder[T any](items []T) *closeRecorder[T] {
	return &closeRecorder[T]{Iterator: iterators.Slice(items)}
}

func (it *closeRecorder[T]) Close() {
	it.closed = true
	it.Iterator.Close()
}

// recvStream is a minimal stand-in for a gRPC stream, for Stream().
type recvStream[T any] struct {
	items      []T
	i          int
	err        error
	closeErr   error
	closeCalls int
}

func (s *recvStream[T]) Recv() (T, error) {
	var zero T
	if s.i >= len(s.items) {
		if s.err != nil {
			return zero, s.err
		}
		return zero, nil
	}
	item := s.items[s.i]
	s.i++
	return item, nil
}

func (s *recvStream[T]) CloseSend() error {
	s.closeCalls++
	return s.closeErr
}
