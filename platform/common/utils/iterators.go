/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

type Iterator[V any] interface {
	// Next returns the next item in the result set. The `QueryResult` is expected to be nil when
	// the iterator gets exhausted
	Next() (V, error)

	// Close releases resources occupied by the iterator
	Close()
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
		return Zero[B](), err
	} else {
		return it.transformer(next)
	}
}

func NewEmptyIterator[K any]() *emptyIterator[K] { return &emptyIterator[K]{zero: Zero[K]()} }

type emptyIterator[K any] struct{ zero K }

func (i *emptyIterator[K]) HasNext() bool { return false }

func (i *emptyIterator[K]) Close() {}

func (i *emptyIterator[K]) Next() (K, error) { return i.zero, nil }
