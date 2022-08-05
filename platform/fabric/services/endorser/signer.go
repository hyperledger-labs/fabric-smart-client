/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

type SerializableSigner interface {
	Sign(message []byte) ([]byte, error)

	Serialize() ([]byte, error)
}
