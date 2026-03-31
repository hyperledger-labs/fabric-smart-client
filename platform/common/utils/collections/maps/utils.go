/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package maps

// Inverse creates a map by inversing the keys with the values to enable searching a key by the value
func Inverse[K comparable, V comparable](in map[K]V) map[V]K {
	out := make(map[V]K, len(in))
	for k, v := range in {
		out[v] = k
	}
	return out
}

// SubMap returns a new map that contains only the key-values of the input map that correspond to the input keys
func SubMap[K comparable, V any](m map[K]V, ks ...K) (map[K]V, []K) {
	found := make(map[K]V, len(ks))
	notFound := make([]K, 0, len(ks))
	for _, k := range ks {
		if v, ok := m[k]; ok {
			found[k] = v
		} else {
			notFound = append(notFound, k)
		}
	}
	return found, notFound
}

// RepeatValue creates a map where the input keys map to the same value
func RepeatValue[K comparable, V any](keys []K, val V) map[K]V {
	res := make(map[K]V, len(keys))
	for _, k := range keys {
		res[k] = val
	}
	return res
}
