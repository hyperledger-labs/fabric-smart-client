/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"crypto/sha256"
	"encoding/hex"
)

func computeInternalSessionID(topic string, pkid []byte) string {
	h := sha256.Sum256(pkid)
	return topic + "." + hex.EncodeToString(h[:])
}
