/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package functions

type Mapper[A any, B any] func(A) (B, error)

type Filter[T any] func(T) bool

func Not[T any](f Filter[T]) Filter[T] {
	return func(t T) bool { return !f(t) }
}
