/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

// Filter lazily filters an iterator
func Filter[A any](iterator Iterator[*A], filter Predicate[*A]) Iterator[*A] {
	return &filtered[A]{Iterator: iterator, filter: filter}
}

type filtered[A any] struct {
	Iterator[*A]
	filter Predicate[*A]
}

func (it *filtered[A]) Next() (*A, error) {
	next, err := it.Iterator.Next()
	if err != nil {
		return nil, err
	}
	if next == nil {
		return nil, nil
	}
	if !it.filter(next) {
		return it.Next()
	}
	return next, nil
}
