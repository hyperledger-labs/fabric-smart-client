/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

func IgnoreErrorFunc(f func() error) {
	_ = f()
}

func IgnoreErrorWithOneArg[T any](fn func(t T) error, t T) {
	_ = fn(t)
}
