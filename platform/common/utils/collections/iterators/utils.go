/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"

// Copy returns a copy of the input Iterator
func Copy[T any](it Iterator[*T]) (Iterator[*T], error) {
	all, err := ReadAllPointers(it)
	if err != nil {
		return nil, err
	}
	return Slice(all), nil
}

// ReadAllPointers reads all pointer elements of an Iterator and returns them
func ReadAllPointers[T any](it Iterator[*T]) ([]*T, error) {
	defer it.Close()
	items := make([]*T, 0)
	for item, err := it.Next(); item != nil || err != nil; item, err = it.Next() {
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// ReadAllValues reads all pointer elements of an Iterator and returns the values
// No nil elements are expected!
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

// ReadFirst reads the first {{limit}} elements of the input Iterator
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

// GetUnique returns the unique element of an Iterator, when there is supposed to be only one
func GetUnique[T any](vs Iterator[T]) (T, error) {
	defer vs.Close()
	return vs.Next()
}

// GetFirst returns the first element of an Iterator, when there may be more than one
func GetFirst[T any](vs Iterator[T]) (T, error) {
	defer vs.Close()
	return vs.Next()
}

// Reduce reduces the elements of an iterator into an aggregated structure
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

// ForEach executes the given ConsumeFunc for each element of the Iterator
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
