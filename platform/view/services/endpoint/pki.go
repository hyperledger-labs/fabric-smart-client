/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"crypto/sha256"
	"crypto/x509"
)

type DefaultPublicKeyIDSynthesizer struct{}

func (d DefaultPublicKeyIDSynthesizer) PublicKeyID(key any) []byte {
	raw, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil
	}
	h := sha256.Sum256(raw)
	return h[:]
}
