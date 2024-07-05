/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hash

import (
	"hash"
)

type Hasher interface {
	GetHash() hash.Hash

	Hash(msg []byte) ([]byte, error)
}
