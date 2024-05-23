/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

type Set[K comparable] map[K]struct{}

func NewSet[K comparable](items ...K) Set[K] {
	set := make(Set[K], len(items))
	for _, item := range items {
		set[item] = struct{}{}
	}
	return set
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

func (s *Set[K]) Contains(key K) bool {
	_, ok := (*s)[key]
	return ok
}

func (s *Set[K]) Empty() bool {
	return len(*s) == 0
}

func Keys[K comparable, V any](m map[K]V) []K {
	res := make([]K, len(m))
	i := 0
	for k := range m {
		res[i] = k
		i++
	}

	return res
}

func Values[K comparable, V any](m map[K]V) []V {
	res := make([]V, len(m))
	i := 0
	for _, v := range m {
		res[i] = v
		i++
	}

	return res
}

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

func InverseMap[K comparable, V comparable](in map[K]V) map[V]K {
	out := make(map[V]K, len(in))
	for k, v := range in {
		out[v] = k
	}
	return out
}
