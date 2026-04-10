/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"crypto/sha256"
	"crypto/x509"
)

// DefaultPublicKeyIDSynthesizer is the default implementation of PublicKeyIDSynthesizer
// that generates public key IDs using SHA-256 hashing of the PKIX-encoded public key.
type DefaultPublicKeyIDSynthesizer struct{}

// PublicKeyID generates a unique identifier for a public key by marshaling it to
// PKIX format and computing its SHA-256 hash.
// Returns an error if the key cannot be marshaled (e.g., unsupported key type).
func (d DefaultPublicKeyIDSynthesizer) PublicKeyID(key any) ([]byte, error) {
	raw, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(raw)
	return h[:], nil
}
