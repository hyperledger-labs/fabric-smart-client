/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package collections

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

func Intersection[V comparable](a, b []V) []V {
	//if len(a) > len(b) {
	//	a, b = b, a
	//}
	aSet := NewSet(a...)
	var res []V
	for _, k := range b {
		if aSet.Contains(k) {
			res = append(res, k)
		}
	}
	return res
}

func Repeat[T any](item T, times int) []T {
	items := make([]T, times)
	for i := 0; i < times; i++ {
		items[i] = item
	}
	return items
}
