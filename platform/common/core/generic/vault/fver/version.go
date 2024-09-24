/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fver

import (
	"bytes"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

var zeroVersion = []byte{0, 0, 0, 0, 0, 0, 0, 0}

func IsEqual(a, b driver.RawVersion) bool {
	if bytes.Equal(a, b) {
		return true
	}
	if len(a) == 0 && bytes.Equal(zeroVersion, b) {
		return true
	}
	if len(b) == 0 && bytes.Equal(zeroVersion, a) {
		return true
	}
	return false
}
