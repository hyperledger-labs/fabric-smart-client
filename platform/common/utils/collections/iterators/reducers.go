/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/sets"

func ToSet[V comparable]() Reducer[*V, sets.Set[V]] { return &setReducer[V]{} }

type setReducer[V comparable] struct{}

func (r *setReducer[V]) Produce() sets.Set[V] { return sets.New[V]() }

func (r *setReducer[V]) Reduce(s sets.Set[V], v *V) (sets.Set[V], error) {
	s.Add(*v)
	return s, nil
}

func ToFlattened[V any]() Reducer[*[]V, []V] { return &flatReducer[V]{} }

type flatReducer[V any] struct{}

func (r *flatReducer[V]) Produce() []V { return []V{} }

func (r *flatReducer[V]) Reduce(vs []V, v *[]V) ([]V, error) { return append(vs, *v...), nil }
