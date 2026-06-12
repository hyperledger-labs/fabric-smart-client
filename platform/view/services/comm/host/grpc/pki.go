/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"

	"github.com/mr-tron/base58/base58"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

type PKIDSynthesizer struct{}

func (p PKIDSynthesizer) PublicKeyID(key any) ([]byte, error) {
	switch d := key.(type) {
	case *ecdsa.PublicKey:
		return ecdsaPubKeyID(d)
	case []byte:
		h := sha256.Sum256(d)
		return h[:], nil
	default:
		return nil, errors.Errorf("unsupported key type [%T]", key)
	}
}

func ecdsaPubKeyID(key *ecdsa.PublicKey) ([]byte, error) {
	marshaledPubKey, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal PK")
	}

	h := sha256.Sum256(marshaledPubKey)
	return []byte(base58.Encode(h[:])), nil
}
