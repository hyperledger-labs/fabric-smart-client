/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/sets"

// NewReducer creates a generic reducer
func NewReducer[V any, S any](initial S, merge ReduceFunc[V, S]) Reducer[V, S] {
	return &reducer[V, S]{initial: initial, merge: merge}
}

type reducer[V any, S any] struct {
	initial S
	merge   ReduceFunc[V, S]
}

func (r *reducer[V, S]) Produce() S { return r.initial }

func (r *reducer[V, S]) Reduce(s S, v V) (S, error) { return r.merge(s, v) }

// ToSet creates a reducer that collects the comparable elements of an Iterator into a Set
func ToSet[V comparable]() Reducer[*V, sets.Set[V]] { return &setReducer[V]{} }

type setReducer[V comparable] struct{}

func (r *setReducer[V]) Produce() sets.Set[V] { return sets.New[V]() }

func (r *setReducer[V]) Reduce(s sets.Set[V], v *V) (sets.Set[V], error) {
	s.Add(*v)
	return s, nil
}

// ToFlattened creates a reducer that collects the slice elements of an Iterator into a flattened slice
func ToFlattened[V any]() Reducer[*[]V, []V] { return &flatReducer[V]{} }

type flatReducer[V any] struct{}

func (r *flatReducer[V]) Produce() []V { return []V{} }

func (r *flatReducer[V]) Reduce(vs []V, v *[]V) ([]V, error) { return append(vs, *v...), nil }
