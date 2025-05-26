/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package collections

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/slices"
)

func Remove[T comparable](items []T, toRemove T) ([]T, bool) { return slices.Remove(items, toRemove) }

func Difference[V comparable](a, b []V) []V { return slices.Difference(a, b) }

func Intersection[V comparable](a, b []V) []V { return slices.Intersection(a, b) }

func Repeat[T any](item T, times int) []T { return slices.Repeat(item, times) }

func GetUnique[T any](vs iterators.Iterator[T]) (T, error) { return iterators.GetUnique(vs) }
