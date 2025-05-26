/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package maps

func Copy[K comparable, V any](to map[K]V, from map[K]V) {
	if from == nil {
		return
	}
	for k, v := range from {
		to[k] = v
	}
}

func Inverse[K comparable, V comparable](in map[K]V) map[V]K {
	out := make(map[V]K, len(in))
	for k, v := range in {
		out[v] = k
	}
	return out
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

func ContainsValue[K, V comparable](haystack map[K]V, needle V) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
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

func RepeatValue[K comparable, V any](keys []K, val V) map[K]V {
	res := make(map[K]V, len(keys))
	for _, k := range keys {
		res[k] = val
	}
	return res
}
