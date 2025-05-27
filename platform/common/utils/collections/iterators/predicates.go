/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

// DuplicatesBy is used for filtering duplicate elements by a comparable property
func DuplicatesBy[V any, I comparable](prop func(V) I) Predicate[V] {
	s := map[I]struct{}{}
	return func(v V) bool {
		k := prop(v)
		if _, ok := s[k]; ok {
			return false
		}
		s[k] = struct{}{}
		return true
	}
}
