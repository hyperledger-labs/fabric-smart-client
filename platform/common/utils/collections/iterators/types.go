/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

type baseIterator[k any] interface {
	// Next returns the next element, or the zero value once the iterator is
	// exhausted. Whether exhaustion is signaled by a nil pointer or by a zero
	// value alongside a nil error depends on the concrete iterator; see its
	// doc comment.
	Next() (k, error)
}

// Iterator produces a sequence of elements one at a time, lazily: an element
// is only computed on the call to Next that yields it. Close must be called
// once the caller is done reading, even after an error or partial read, to
// release any resource (a goroutine, a stream, a file) the iterator holds.
type Iterator[V any] interface {
	baseIterator[V]

	// Close releases resources occupied by the iterator
	Close()
}

// ConsumeFunc processes a single element, for use with ForEach.
type ConsumeFunc[V any] func(V) error

// Reducer folds the elements of an Iterator into a single result of type S,
// starting from Produce and applying Reduce for each element in turn.
type Reducer[V any, S any] interface {
	Produce() S
	Reduce(S, V) (S, error)
}

// ReduceFunc merges one element into the accumulated result, for use with
// ReduceValue.
type ReduceFunc[V any, S any] func(S, V) (S, error)

// Predicate reports whether an element satisfies some condition, for use with
// Filter and DuplicatesBy.
type Predicate[V any] func(V) bool

// Transformer derives a B from an A, for use with Map and Flatten. It may
// fail, since the derivation often involves parsing or another fallible
// operation.
type Transformer[A any, B any] func(A) (B, error)
