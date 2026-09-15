/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

// Empty returns an empty Iterator
func Empty[K any]() Iterator[K] { return &empty[K]{} }

type empty[K any] struct{}

func (*empty[K]) Close() {}

func (*empty[K]) Next() (K, error) {
	var zero K
	return zero, nil
}
