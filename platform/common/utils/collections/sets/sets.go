/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sets

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/maps"

type set[V comparable] map[V]struct{}

func New[V comparable](items ...V) Set[V] {
	s := make(set[V], len(items))
	for _, item := range items {
		s[item] = struct{}{}
	}
	return &s
}

// Add adds an element to the set
func (s *set[V]) Add(vs ...V) {
	for _, v := range vs {
		(*s)[v] = struct{}{}
	}
}

// Remove removes an element from the set
func (s *set[V]) Remove(vs ...V) {
	for _, v := range vs {
		delete(*s, v)
	}
}

// Contains returns true if the passed element is contained in the set
func (s *set[V]) Contains(v V) bool {
	_, ok := (*s)[v]
	return ok
}

// Minus returns all elements that are contained in the set, but are not contained in the input set
func (s *set[V]) Minus(other Set[V]) Set[V] {
	difference := New[V]()
	for k := range *s {
		if !other.Contains(k) {
			difference.Add(k)
		}
	}
	return difference
}

// ToSlice reads all set elements into a slice
func (s *set[V]) ToSlice() []V {
	return maps.Keys(*s)
}

// Length returns the size of the set
func (s *set[V]) Length() int {
	return len(*s)
}

// Empty returns true if the set contains no elements
func (s *set[V]) Empty() bool {
	return s.Length() == 0
}

// Set is a collection of unique comparable items
type Set[V comparable] interface {
	Add(...V)
	Remove(...V)
	Minus(Set[V]) Set[V]
	Contains(V) bool
	ToSlice() []V
	Empty() bool
	Length() int
}
