/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

func Copy[T any](it Iterator[*T]) (*slice[*T], error) {
	all, err := ReadAllPointers(it)
	if err != nil {
		return nil, err
	}
	return NewSlice(all), nil
}

func ReadAllPointers[T any](it Iterator[*T]) ([]*T, error) {
	defer it.Close()
	items := make([]*T, 0)
	for item, err := it.Next(); item != nil && err == nil; item, err = it.Next() {
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func ReadAllValues[T any](it Iterator[*T]) ([]T, error) {
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

func GetUnique[T any](vs Iterator[T]) (T, error) {
	defer vs.Close()
	return vs.Next()
}

type reducer[V any, S any] struct {
	initial S
	merge   ReduceFunc[V, S]
}

func (r *reducer[V, S]) Produce() S { return r.initial }

func (r *reducer[V, S]) Reduce(s S, v V) (S, error) { return r.merge(s, v) }

func NewReducer[V any, S any](initial S, merge ReduceFunc[V, S]) *reducer[V, S] {
	return &reducer[V, S]{initial: initial, merge: merge}
}

func Reduce[V any, S any](it Iterator[*V], reducer Reducer[*V, S]) (S, error) {
	return ReduceValue(it, reducer.Produce(), reducer.Reduce)
}

func ReduceValue[V any, S any](it Iterator[*V], result S, reduce ReduceFunc[*V, S]) (S, error) {
	defer it.Close()
	for {
		item, err := it.Next()
		if err != nil {
			return utils.Zero[S](), err
		}
		if item == nil {
			return result, nil
		}
		result, err = reduce(result, item)
		if err != nil {
			return utils.Zero[S](), err
		}
	}
}

func ForEach[V any](it Iterator[*V], consume ConsumeFunc[*V]) error {
	defer it.Close()
	for {
		item, err := it.Next()
		if err != nil {
			return err
		}
		if item == nil {
			return nil
		}
		if err := consume(item); err != nil {
			return err
		}
	}
}
