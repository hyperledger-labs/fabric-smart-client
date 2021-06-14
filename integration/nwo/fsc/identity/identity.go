/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package identity

import (
	"crypto/ecdsa"
)

type Identity interface {
	Serialize() ([]byte, error)
}

type SigningIdentity interface {
	Identity
	Sign(msg []byte) ([]byte, error)
}

type signingIdentity struct {
	serialized []byte
	edsaSigner *edsaSigner
}

func NewSigningIdentity(serialized []byte, privateKey *ecdsa.PrivateKey) *signingIdentity {
	return &signingIdentity{
		serialized: serialized,
		edsaSigner: &edsaSigner{sk: privateKey},
	}
}

func (s *signingIdentity) Serialize() ([]byte, error) {
	return s.serialized, nil
}

func (s *signingIdentity) Sign(message []byte) ([]byte, error) {
	return s.edsaSigner.Sign(message)
}
