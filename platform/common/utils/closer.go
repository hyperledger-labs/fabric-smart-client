/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import "fmt"

type Closer interface {
	Close() error
}

func CloseMute(closer Closer) {
	if closer == nil {
		return
	}
	// ignore err
	_ = closer.Close()
}

func IgnoreError(err error) {
	// ignore the error
	fmt.Printf("IgnoreError: %v\n", err)
}

func IgnoreErrorFunc(f func() error) {
	_ = f()
}

func IgnoreErrorWithOneArg[T any](fn func(t T) error, t T) {
	_ = fn(t)
}
