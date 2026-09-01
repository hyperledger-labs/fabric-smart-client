/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

import (
	"cmp"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/sets"
)

// NewReducer creates a generic reducer
func NewReducer[V, S any](initial S, merge ReduceFunc[V, S]) Reducer[V, S] {
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

// ToMaxBy returns a [Reducer] that selects the element with the greatest key, as
// derived by fn. Ties keep the earlier element, and an empty [Iterator] reduces
// to the zero value of V. The returned [Reducer] is stateful: use a new one for
// each reduction.
func ToMaxBy[V any, K cmp.Ordered](fn Transformer[V, K]) Reducer[V, V] {
	return &maxByReducer[V, K]{fn: fn}
}

type maxByReducer[V any, K cmp.Ordered] struct {
	fn     Transformer[V, K]
	maxKey K
	// seen distinguishes the first element from a later one, so that keys below
	// the zero value of K can still win.
	seen bool
}

func (r *maxByReducer[V, K]) Produce() V { return utils.Zero[V]() }

func (r *maxByReducer[V, K]) Reduce(maxVal, v V) (V, error) {
	currKey, err := r.fn(v)
	if err != nil {
		return utils.Zero[V](), err
	}
	if r.seen && currKey <= r.maxKey {
		return maxVal, nil
	}
	r.maxKey = currKey
	r.seen = true
	return v, nil
}
