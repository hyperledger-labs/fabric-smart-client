/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package collections

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/slices"
)

// Remove removes the first occurrence of toRemove from items. See
// [slices.Remove].
func Remove[T comparable](items []T, toRemove T) ([]T, bool) { return slices.Remove(items, toRemove) }

// Difference returns the elements of a that are not in b. See
// [slices.Difference].
func Difference[V comparable](a, b []V) []V { return slices.Difference(a, b) }

// Intersection returns the elements contained in both a and b. See
// [slices.Intersection].
func Intersection[V comparable](a, b []V) []V { return slices.Intersection(a, b) }

// Repeat returns a slice with item repeated times times. See
// [slices.Repeat].
func Repeat[T any](item T, times int) []T { return slices.Repeat(item, times) }

// GetUnique returns the single element of vs, when there is supposed to be
// only one. See [iterators.GetUnique].
func GetUnique[T any](vs iterators.Iterator[T]) (T, error) { return iterators.GetUnique(vs) }
