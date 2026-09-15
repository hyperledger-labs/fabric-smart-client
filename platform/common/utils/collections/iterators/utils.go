/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

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

// ReadFirst reads at most the first limit elements of the [Iterator] and closes
// it. No element beyond limit is read, and a limit of zero or less reads none.
func ReadFirst[T any](it Iterator[*T], limit int) ([]T, error) {
	defer it.Close()
	items := make([]T, 0)
	for len(items) < limit {
		item, err := it.Next()
		if err != nil {
			return nil, err
		}
		if item == nil {
			return items, nil
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
//
//nolint:revive // confusing-naming: package func Reduce and the Reducer.Reduce method are both exported public API; renaming either is an API break; see follow-up
func Reduce[V, S any](it Iterator[*V], reducer Reducer[*V, S]) (S, error) {
	return ReduceValue(it, reducer.Produce(), reducer.Reduce)
}

// ReduceValue folds the elements of it into result by applying reduce to each
// in turn, starting from the given initial result. It underlies [Reduce],
// exposed separately for callers that build up a result without a [Reducer].
func ReduceValue[V, S any](it Iterator[*V], result S, reduce ReduceFunc[*V, S]) (S, error) {
	defer it.Close()
	var zero S
	for {
		item, err := it.Next()
		if err != nil {
			return zero, err
		}
		if item == nil {
			return result, nil
		}
		result, err = reduce(result, item)
		if err != nil {
			return zero, err
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
