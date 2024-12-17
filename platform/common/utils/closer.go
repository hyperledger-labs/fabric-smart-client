/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

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

func IgnoreError(_ error) {}
