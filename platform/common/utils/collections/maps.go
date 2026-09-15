/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package collections

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/maps"

// CopyMap copies the elements of from into to. See [maps.Copy].
func CopyMap[K comparable, V any](to, from map[K]V) { maps.Copy(to, from) }

// InverseMap swaps keys and values, to enable searching a key by its value.
// See [maps.Inverse].
func InverseMap[K, V comparable](in map[K]V) map[V]K { return maps.Inverse(in) }

// Values returns all values of m, in no particular order. See [maps.Values].
func Values[K comparable, V any](m map[K]V) []V { return maps.Values(m) }

// ContainsValue reports whether needle is a value in haystack. See
// [maps.ContainsValue].
func ContainsValue[K, V comparable](haystack map[K]V, needle V) bool {
	return maps.ContainsValue(haystack, needle)
}

// Keys returns all keys of m, in no particular order. See [maps.Keys].
func Keys[K comparable, V any](m map[K]V) []K { return maps.Keys(m) }

// SubMap returns the entries of m keyed by ks, and the ks not found in m.
// See [maps.SubMap].
func SubMap[K comparable, V any](m map[K]V, ks ...K) (map[K]V, []K) { return maps.SubMap(m, ks...) }

// RepeatValue returns a map where every key in keys maps to val. See
// [maps.RepeatValue].
func RepeatValue[K comparable, V any](keys []K, val V) map[K]V { return maps.RepeatValue(keys, val) }
