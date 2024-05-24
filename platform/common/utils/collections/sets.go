/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package collections

type set[V comparable] map[V]struct{}

func NewSet[V comparable](items ...V) Set[V] {
	s := make(set[V], len(items))
	for _, item := range items {
		s[item] = struct{}{}
	}
	return &s
}

func (s *set[V]) Add(vs ...V) {
	for _, v := range vs {
		(*s)[v] = struct{}{}
	}
}

func (s *set[V]) Contains(v V) bool {
	_, ok := (*s)[v]
	return ok
}

func (s *set[V]) Minus(other Set[V]) Set[V] {
	difference := NewSet[V]()
	for k := range *s {
		if !other.Contains(k) {
			difference.Add(k)
		}
	}
	return difference
}

func (s *set[V]) ToSlice() []V {
	return Keys(*s)
}

func (s *set[V]) Length() int {
	return len(*s)
}
func (s *set[V]) Empty() bool {
	return s.Length() == 0
}

type Set[V comparable] interface {
	Add(...V)
	Minus(Set[V]) Set[V]
	Contains(V) bool
	ToSlice() []V
	Empty() bool
	Length() int
}
