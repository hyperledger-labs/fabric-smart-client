/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

func Zero[A any]() A {
	var a A
	return a
}

func Must(err error) {
	if err != nil {
		panic(err)
	}
}

func DefaultString(a, b string) string {
	if len(a) > 0 {
		return a
	}
	return b
}
