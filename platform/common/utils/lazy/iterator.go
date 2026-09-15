/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lazy

import "io"

// NewIterator returns an [Iterator] that produces its elements lazily, one
// per call to Next, by calling each of fs in order.
func NewIterator[T any](fs ...func() (T, error)) *Iterator[T] {
	return &Iterator[T]{fs: fs}
}

// Iterator produces a fixed sequence of elements, each computed only when
// Next reaches it rather than up front.
type Iterator[T any] struct {
	fs []func() (T, error)
}

// Next produces the next element by calling its underlying function, or
// returns io.EOF once every element has been produced.
func (it *Iterator[T]) Next() (T, error) {
	if len(it.fs) == 0 {
		var zero T
		return zero, io.EOF
	}
	result, err := it.fs[0]()
	it.fs = it.fs[1:]
	return result, err
}

// Close discards the remaining, not-yet-produced elements.
func (it *Iterator[T]) Close() {
	it.fs = nil
}
