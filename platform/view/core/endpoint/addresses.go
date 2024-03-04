/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

type SelectionStrategy[T any] func([]T) T

func AlwaysFirst[T any]() SelectionStrategy[T] {
	return func(sets []T) T {
		return sets[0]
	}
}
func AlwaysLast[T any]() SelectionStrategy[T] {
	return func(sets []T) T {
		return sets[len(sets)-1]
	}
}
