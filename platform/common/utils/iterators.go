/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

type Iterator[V any] interface {
	Next() (V, error)
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
