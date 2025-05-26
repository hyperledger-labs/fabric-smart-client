/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

import (
	"math/rand"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

func NewPermutated[T any](it Iterator[*T]) (*slice[*T], error) {
	items, err := ReadAllPointers(it)
	if err != nil {
		return nil, err
	}
	rand.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	return NewSlice(items), nil
}

func NewSingle[T any](item T) *slice[T] {
	return NewSlice[T]([]T{item})
}

type slice[T any] struct {
	i     int
	items []T
}

func NewSlice[T any](items []T) *slice[T] { return &slice[T]{items: items} }

func (it *slice[T]) Next() (T, error) {
	if !it.HasNext() {
		return utils.Zero[T](), nil
	}
	item := it.items[it.i]
	it.i++
	return item, nil
}

func (it *slice[K]) HasNext() bool { return it.i < len(it.items) }

func (it *slice[T]) Close() {
	it.items = nil
}

func (it *slice[T]) NewPermutation() Iterator[T] {
	return &permutation[T]{
		items: it.items,
		perm:  rand.Perm(len(it.items)),
	}
}

type permutation[T any] struct {
	i     int
	items []T
	perm  []int
}

func (it *permutation[T]) Next() (T, error) {
	if !it.HasNext() {
		return utils.Zero[T](), nil
	}
	item := it.items[it.perm[it.i]]
	it.i++
	return item, nil
}

func (it *permutation[T]) HasNext() bool { return it.i < len(it.items) }

func (it *permutation[T]) Close() {
	it.items = nil
	it.perm = nil
}
