/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package collections re-exports a subset of the iterators, maps, sets and
// slices subpackages under shorter names, for callers that only need a
// handful of these functions and don't want to import each subpackage
// separately. Callers needing the rest of a subpackage's API import it
// directly instead.
package collections

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
)

// Iterator is an alias for [iterators.Iterator].
type Iterator[V any] iterators.Iterator[V]

// NewPermutatedIterator returns an [Iterator] that yields the elements of it
// in a random permutation. See [iterators.Permutate].
func NewPermutatedIterator[T any](it iterators.Iterator[*T]) (iterators.Iterator[*T], error) {
	return iterators.Permutate(it)
}

// CopyIterator returns an independent copy of it, materializing its
// remaining elements. See [iterators.Copy].
func CopyIterator[T any](it iterators.Iterator[*T]) (iterators.Iterator[*T], error) {
	return iterators.Copy(it)
}

// ReadFirst reads at most the first limit elements of it and closes it. See
// [iterators.ReadFirst].
func ReadFirst[T any](it iterators.Iterator[*T], limit int) ([]T, error) {
	return iterators.ReadFirst(it, limit)
}

// ReadAll reads every element of it into a slice and closes it. See
// [iterators.ReadAllValues].
func ReadAll[T any](it iterators.Iterator[*T]) ([]T, error) {
	return iterators.ReadAllValues(it)
}

// NewSingleIterator returns an [Iterator] that yields item once. See
// [iterators.From].
func NewSingleIterator[T any](item T) iterators.Iterator[T] {
	return iterators.From(item)
}

// NewSliceIterator returns an [Iterator] over items, in order. See
// [iterators.Slice].
func NewSliceIterator[T any](items []T) iterators.Iterator[T] {
	return iterators.Slice(items)
}

// Map returns an [Iterator] that applies transformer to each element of
// iterator lazily. See [iterators.Map].
func Map[A, B any](iterator iterators.Iterator[A], transformer func(A) (B, error)) iterators.Iterator[B] {
	return iterators.Map(iterator, transformer)
}

// Filter returns an [Iterator] that yields only the elements of iterator
// satisfying filter. See [iterators.Filter].
func Filter[A any](iterator iterators.Iterator[*A], filter iterators.Predicate[*A]) iterators.Iterator[*A] {
	return iterators.Filter(iterator, filter)
}

// NewEmptyIterator returns an [Iterator] that yields no elements. See
// [iterators.Empty].
func NewEmptyIterator[K any]() iterators.Iterator[K] { return iterators.Empty[K]() }
