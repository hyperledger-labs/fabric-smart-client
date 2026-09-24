/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import "slices"

type Namespaces []string

func (k Namespaces) Count() int {
	return len(k)
}

func (k Namespaces) Match(keys Namespaces) bool {
	if len(k) != len(keys) {
		return false
	}
	for _, id := range k {
		found := slices.Contains(keys, id)
		if !found {
			return false
		}
	}

	return true
}

func (k Namespaces) Filter(f func(k string) bool) Namespaces {
	var filtered Namespaces
	for _, output := range k {
		if f(output) {
			filtered = append(filtered, output)
		}
	}
	return filtered
}

// At returns the namespace at the passed position, or the empty string if it is
// out of range.
func (k Namespaces) At(i int) string {
	if i < 0 || i >= len(k) {
		return ""
	}
	return k[i] //nolint:gosec // G602: i is bounds-checked against len(k) directly above
}
