/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package maps

// Copy copies the elements of the second map to the first
func Copy[K comparable, V any](to map[K]V, from map[K]V) {
	if from == nil {
		return
	}
	for k, v := range from {
		to[k] = v
	}
}

// Inverse creates a map by inversing the keys with the values to enable searching a key by the value
func Inverse[K comparable, V comparable](in map[K]V) map[V]K {
	out := make(map[V]K, len(in))
	for k, v := range in {
		out[v] = k
	}
	return out
}

// Values returns all values of the input map
func Values[K comparable, V any](m map[K]V) []V {
	res := make([]V, len(m))
	i := 0
	for _, v := range m {
		res[i] = v
		i++
	}

	return res
}

// Keys returns all keys of the input map
func Keys[K comparable, V any](m map[K]V) []K {
	res := make([]K, len(m))
	i := 0
	for k := range m {
		res[i] = k
		i++
	}

	return res
}

// ContainsValue scans the comparable values of a map for the input value
func ContainsValue[K, V comparable](haystack map[K]V, needle V) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
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
