/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

type SigningIdentity interface {
	Serialize() ([]byte, error)
	Sign(msg []byte) ([]byte, error)
}
