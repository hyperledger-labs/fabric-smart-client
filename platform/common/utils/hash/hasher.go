/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hash

type Hasher interface {
	Hash(msg []byte) ([]byte, error)
}
