/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

// DuplicatesBy returns a stateful [Predicate] that keeps the first element
// seen for each key prop derives and rejects every later element with a key
// already seen. Reuse it only across a single pass: the seen set never
// shrinks.
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

// Or returns a [Predicate] that accepts a value when either this or that does.
func Or[A any](this, that Predicate[A]) Predicate[A] {
	return func(v A) bool { return this(v) || that(v) }
}
