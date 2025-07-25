/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package slices

import (
	"slices"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/sets"
	"golang.org/x/exp/constraints"
)

// Remove removes the first occurrence of the input item from the input slice
func Remove[T comparable](items []T, toRemove T) ([]T, bool) {
	if items == nil {
		return nil, false
	}
	for i, l := range items {
		if l == toRemove {
			return append(items[:i], items[i+1:]...), true
		}
	}
	return items, false
}

// Difference returns a slice that contains all elements of the first input slice without the elements of the second input slice
func Difference[V comparable](a, b []V) []V {
	return sets.New(a...).Minus(sets.New(b...)).ToSlice()
}

// Intersection returns a slice that contains all elements that are contained in both slices
func Intersection[V comparable](a, b []V) []V {
	//if len(a) > len(b) {
	//	a, b = b, a
	//}
	aSet := sets.New(a...)
	var res []V
	for _, k := range b {
		if aSet.Contains(k) {
			res = append(res, k)
		}
	}
	return res
}

// Repeat returns a slice with the same element repeated {{times}} times
func Repeat[T any](item T, times int) []T {
	items := make([]T, times)
	for i := 0; i < times; i++ {
		items[i] = item
	}
	return items
}

type SortedSlice[T constraints.Ordered] []T

func (s *SortedSlice[T]) Add(t T) {
	if i, found := slices.BinarySearch(*s, t); !found {
		*s = append(*s, t)
		copy((*s)[i+1:], (*s)[i:])
		(*s)[i] = t
	}
}
