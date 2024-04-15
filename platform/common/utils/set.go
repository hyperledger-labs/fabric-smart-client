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

func (s *Set[K]) Contains(key K) bool {
	_, ok := (*s)[key]
	return ok
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
