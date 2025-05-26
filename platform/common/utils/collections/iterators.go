/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package collections

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
)

type Iterator[V any] iterators.Iterator[V]

func NewPermutatedIterator[T any](it iterators.Iterator[*T]) (iterators.Iterator[*T], error) {
	return iterators.NewPermutated(it)
}

func CopyIterator[T any](it iterators.Iterator[*T]) (iterators.Iterator[*T], error) {
	return iterators.Copy(it)
}

func ReadFirst[T any](it iterators.Iterator[*T], limit int) ([]T, error) {
	return iterators.ReadFirst(it, limit)
}

func ReadAll[T any](it iterators.Iterator[*T]) ([]T, error) {
	return iterators.ReadAllValues(it)
}

func NewSingleIterator[T any](item T) iterators.Iterator[T] {
	return iterators.NewSingle(item)
}

func NewSliceIterator[T any](items []T) iterators.Iterator[T] {
	return iterators.NewSlice(items)
}

func Map[A any, B any](iterator iterators.Iterator[A], transformer func(A) (B, error)) iterators.Iterator[B] {
	return iterators.Map(iterator, transformer)
}

func Filter[A any](iterator iterators.Iterator[*A], filter iterators.Predicate[*A]) iterators.Iterator[*A] {
	return iterators.Filter(iterator, filter)
}

func NewEmptyIterator[K any]() iterators.Iterator[K] { return iterators.NewEmpty[K]() }
