/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package identity

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
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
	h := sha256.Sum256(id)
	return base64.StdEncoding.EncodeToString(h[:])
}

// Hash returns the hash of this identity
func (id Identity) Hash() string {
	if len(id) == 0 {
		return "<empty>"
	}
	h := sha256.Sum256(id)
	return string(h[:])
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
