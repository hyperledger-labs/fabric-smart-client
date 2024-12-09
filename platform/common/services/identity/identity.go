/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package identity

import (
	"bytes"
	"encoding/base64"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

// Identity wraps the byte representation of a lower level identity.
type Identity []byte

// Equal return true if the identities are the same
func (id Identity) Equal(id2 Identity) bool {
	return bytes.Equal(id, id2)
}

// UniqueID returns a unique identifier of this identity
func (id Identity) UniqueID() string {
	if len(id) == 0 {
		return "<empty>"
	}
	return base64.StdEncoding.EncodeToString(utils.MustGet(utils.SHA256(id)))
}

// Hash returns the hash of this identity
func (id Identity) Hash() string {
	if len(id) == 0 {
		return "<empty>"
	}
	return string(utils.MustGet(utils.SHA256(id)))
}

// String returns a string representation of this identity
func (id Identity) String() string {
	return id.UniqueID()
}

// Bytes returns the byte representation of this identity
func (id Identity) Bytes() []byte {
	return id
}

// IsNone returns true if this identity is empty
func (id Identity) IsNone() bool {
	return len(id) == 0
}
