/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

type Namespaces []string

func (k Namespaces) Count() int {
	return len(k)
}

func (k Namespaces) Match(keys Namespaces) bool {
	if len(k) != len(keys) {
		return false
	}
	for _, id := range k {
		found := false
		for _, identity := range keys {
			if identity == id {
				found = true
				break
			}
		}
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

func (k Namespaces) At(i int) string {
	return k[i]
}
