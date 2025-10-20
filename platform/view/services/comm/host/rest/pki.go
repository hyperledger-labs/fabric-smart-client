/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/mr-tron/base58/base58"
)

type PKIDSynthesizer struct{}

func (p PKIDSynthesizer) PublicKeyID(key any) []byte {
	switch d := key.(type) {
	case *ecdsa.PublicKey:
		id, err := ecdsaPubKeyID(d)
		if err != nil {
			logger.Errorf("failed to calculate ID of PK: %v", err)
		}
		return id
	case []byte:
		h := sha256.Sum256(d)
		return h[:]
	}
	panic("unsupported key")
}

func ecdsaPubKeyID(key *ecdsa.PublicKey) ([]byte, error) {
	marshaledPubKey, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal PK")
	}

	h := sha256.Sum256(marshaledPubKey)
	return []byte(base58.Encode(h[:])), nil
}
