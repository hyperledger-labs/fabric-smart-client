/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package collections

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"

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

func CopyIterator[T any](it Iterator[*T]) (*sliceIterator[*T], error) {
	defer it.Close()
	items := make([]*T, 0)
	for item, err := it.Next(); item != nil && err == nil; item, err = it.Next() {
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return NewSliceIterator(items), nil
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

func (it *sliceIterator[T]) Close() {}

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

func NewEmptyIterator[K any]() *emptyIterator[K] { return &emptyIterator[K]{zero: utils.Zero[K]()} }

type emptyIterator[K any] struct{ zero K }

func (i *emptyIterator[K]) HasNext() bool { return false }

func (i *emptyIterator[K]) Close() {}

func (i *emptyIterator[K]) Next() (K, error) { return i.zero, nil }
