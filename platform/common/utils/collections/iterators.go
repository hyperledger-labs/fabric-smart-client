/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package collections

import (
	"math/rand"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/functions"
)

type baseIterator[k any] interface {
	// Next returns the next item in the result set. The `QueryResult` is expected to be nil when
	// the iterator gets exhausted
	Next() (k, error)
}

type Iterator[V any] interface {
	baseIterator[V]

	// Close releases resources occupied by the iterator
	Close()
}

func NewPermutatedIterator[T any](it Iterator[*T]) (*sliceIterator[*T], error) {
	items := readAll(it)
	rand.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	return NewSliceIterator(items), nil
}

func CopyIterator[T any](it Iterator[*T]) (*sliceIterator[*T], error) {
	return NewSliceIterator(readAll(it)), nil
}

func readAll[T any](it Iterator[*T]) []*T {
	defer it.Close()
	items := make([]*T, 0)
	for item, err := it.Next(); item != nil && err == nil; item, err = it.Next() {
		items = append(items, item)
	}
	return items
}

func ReadFirst[T any](it Iterator[*T], limit int) ([]T, error) {
	defer it.Close()
	items := make([]T, 0)
	for item, err := it.Next(); (item != nil || err != nil) && len(items) < limit; item, err = it.Next() {
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, nil
}

func ReadAll[T any](it Iterator[*T]) ([]T, error) {
	defer it.Close()
	items := make([]T, 0)
	for item, err := it.Next(); item != nil || err != nil; item, err = it.Next() {
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, nil
}

func ToSlice[T any](it Iterator[*T]) ([]*T, error) {
	defer it.Close()
	items := make([]*T, 0)
	for item, err := it.Next(); item != nil || err != nil; item, err = it.Next() {
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func NewSingleIterator[T any](item T) *sliceIterator[T] {
	return NewSliceIterator[T]([]T{item})
}

type sliceIterator[T any] struct {
	i     int
	items []T
}

func NewSliceIterator[T any](items []T) *sliceIterator[T] { return &sliceIterator[T]{items: items} }

func (it *sliceIterator[T]) Next() (T, error) {
	if !it.HasNext() {
		return utils.Zero[T](), nil
	}
	item := it.items[it.i]
	it.i++
	return item, nil
}

func (it *sliceIterator[K]) HasNext() bool { return it.i < len(it.items) }

func (it *sliceIterator[T]) Close() {
	it.items = nil
}

func (it *sliceIterator[T]) NewPermutation() Iterator[T] {
	return &permutationIterator[T]{
		items: it.items,
		perm:  rand.Perm(len(it.items)),
	}
}

type permutationIterator[T any] struct {
	i     int
	items []T
	perm  []int
}

func (it *permutationIterator[T]) Next() (T, error) {
	if !it.HasNext() {
		return utils.Zero[T](), nil
	}
	item := it.items[it.perm[it.i]]
	it.i++
	return item, nil
}

func (it *permutationIterator[T]) HasNext() bool { return it.i < len(it.items) }

func (it *permutationIterator[T]) Close() {
	it.items = nil
	it.perm = nil
}

func Filter[A any](iterator Iterator[A], filter functions.Filter[A]) Iterator[A] {
	return &filteredIterator[A]{Iterator: iterator, filter: filter}
}

type filteredIterator[A any] struct {
	Iterator[A]
	filter func(A) bool
}

func (it *filteredIterator[A]) Next() (A, error) {
	if next, err := it.Iterator.Next(); err != nil {
		return next, err
	} else if utils.IsNil(next) {
		return utils.Zero[A](), nil
	} else if !it.filter(next) {
		return it.Next()
	} else {
		return next, nil
	}
}

func Map[A any, B any](iterator Iterator[A], transformer functions.Mapper[A, B]) Iterator[B] {
	return &mappedIterator[A, B]{Iterator: iterator, transformer: transformer}
}

type mappedIterator[A any, B any] struct {
	Iterator[A]
	transformer functions.Mapper[A, B]
}

func (it *mappedIterator[A, B]) Next() (B, error) {
	if next, err := it.Iterator.Next(); err != nil {
		return utils.Zero[B](), err
	} else {
		return it.transformer(next)
	}
}

func NewEmptyIterator[K any]() *emptyIterator[K] { return &emptyIterator[K]{zero: utils.Zero[K]()} }

type emptyIterator[K any] struct{ zero K }

func (i *emptyIterator[K]) HasNext() bool { return false }

func (i *emptyIterator[K]) Close() {}

func (i *emptyIterator[K]) Next() (K, error) { return i.zero, nil }
