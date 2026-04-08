/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

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

// GetUnique returns the unique element of an Iterator, when there is supposed to be only one
func GetUnique[T any](vs Iterator[T]) (T, error) {
	defer vs.Close()
	return vs.Next()
}
