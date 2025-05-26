/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package collections

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/maps"

func CopyMap[K comparable, V any](to map[K]V, from map[K]V) { maps.Copy(to, from) }

func InverseMap[K comparable, V comparable](in map[K]V) map[V]K { return maps.Inverse(in) }

func Values[K comparable, V any](m map[K]V) []V { return maps.Values(m) }

func ContainsValue[K, V comparable](haystack map[K]V, needle V) bool {
	return maps.ContainsValue(haystack, needle)
}

func Keys[K comparable, V any](m map[K]V) []K { return maps.Keys(m) }

func SubMap[K comparable, V any](m map[K]V, ks ...K) (map[K]V, []K) { return maps.SubMap(m, ks...) }

func RepeatValue[K comparable, V any](keys []K, val V) map[K]V { return maps.RepeatValue(keys, val) }
