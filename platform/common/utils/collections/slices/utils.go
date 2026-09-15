/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package slices

import (
	"cmp"
	"slices"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/sets"
)

// Remove removes the first occurrence of the input item from the input slice
func Remove[T comparable](items []T, toRemove T) ([]T, bool) {
	i := slices.Index(items, toRemove)
	if i < 0 {
		return items, false
	}
	return slices.Delete(items, i, i+1), true
}

// Difference returns a slice that contains all elements of the first input slice without the elements of the second input slice
func Difference[V comparable](a, b []V) []V {
	return sets.New(a...).Minus(sets.New(b...)).ToSlice()
}

// Intersection returns a slice that contains all elements that are contained in both slices
func Intersection[V comparable](a, b []V) []V {
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
	return slices.Repeat([]T{item}, times)
}

// SortedSlice is a slice kept in ascending order by [SortedSlice.Add]. The
// zero value is an empty sorted slice, ready to use.
type SortedSlice[T cmp.Ordered] []T

// Add inserts t at its sorted position. If an equal element is already
// present, it does nothing: a SortedSlice holds no duplicates.
func (s *SortedSlice[T]) Add(t T) {
	if i, found := slices.BinarySearch(*s, t); !found {
		*s = append(*s, t)
		copy((*s)[i+1:], (*s)[i:])
		(*s)[i] = t
	}
}
