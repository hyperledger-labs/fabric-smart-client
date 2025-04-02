/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package collections

import (
	"math/rand"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

type baseIterator[k any] interface {
	// Next returns the next item in the result set. The `QueryResult` is expected to be nil when
	// the iterator gets exhausted
	Next() (k, error)
	// // Peek returns the next item in the result set. It does not change the location of the iterator.
	// Peek() (k, error)
	// // Peek returns the last item in the result set. It does not change the location of the iterator.
	// PeekLast() (k, error)
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

// // Return the record the iterator points at. Does not change where the iterator points to
// func Peek[T any](it Iterator[*T]) (*T, error) {
// 	return it.Peek()
// }

// // Return the record the iterator points at. Does not change where the iterator points to
// func PeekLast[T any](it Iterator[*T]) (*T, error) {
// 	return it.PeekLast()
// }

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

func ReadLast[T any](it Iterator[*T]) (*T, error) {
	var last *T
	for item, err := it.Next(); item != nil || err != nil; item, err = it.Next() {
		if err != nil {
			return nil, err
		}
		last = item
	}
	return last, nil
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

// func (it *sliceIterator[T]) Peek() (T, error) {
// 	item := it.items[it.i]
// 	return item, nil
// }

// func (it *sliceIterator[T]) PeekLast() (T, error) {
// 	item := it.items[len(it.items)-1]
// 	return item, nil
// }

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

// func (it *permutationIterator[T]) Peek() (T, error) {
// 	item := it.items[it.perm[it.i]]
// 	return item, nil
// }

// func (it *permutationIterator[T]) PeekLast() (T, error) {
// 	item := it.items[it.perm[len(it.items)]]
// 	return item, nil
// }

func (it *permutationIterator[T]) HasNext() bool { return it.i < len(it.items) }

func (it *permutationIterator[T]) Close() {
	it.items = nil
	it.perm = nil
}

func Map[A any, B any](iterator Iterator[A], transformer func(A) (B, error)) Iterator[B] {
	return &mappedIterator[A, B]{Iterator: iterator, transformer: transformer}
}

type mappedIterator[A any, B any] struct {
	Iterator[A]
	transformer func(A) (B, error)
}

func (it *mappedIterator[A, B]) Next() (B, error) {
	if next, err := it.Iterator.Next(); err != nil {
		return utils.Zero[B](), err
	} else {
		return it.transformer(next)
	}
}

// func (it *mappedIterator[A, B]) Peek() (B, error) {
// 	if p, err := it.Iterator.Peek(); err != nil {
// 		return utils.Zero[B](), err
// 	} else {
// 		return it.transformer(p)
// 	}
// }

// func (it *mappedIterator[A, B]) PeekLast() (B, error) {
// 	if p, err := it.Iterator.PeekLast(); err != nil {
// 		return utils.Zero[B](), err
// 	} else {
// 		return it.transformer(p)
// 	}
// }

func NewEmptyIterator[K any]() *emptyIterator[K] { return &emptyIterator[K]{zero: utils.Zero[K]()} }

type emptyIterator[K any] struct{ zero K }

func (i *emptyIterator[K]) HasNext() bool { return false }

func (i *emptyIterator[K]) Close() {}

func (i *emptyIterator[K]) Next() (K, error) { return i.zero, nil }

// func (i *emptyIterator[K]) Peek() (K, error) { return i.zero, nil }

// func (i *emptyIterator[K]) PeekLast() (K, error) { return i.zero, nil }
