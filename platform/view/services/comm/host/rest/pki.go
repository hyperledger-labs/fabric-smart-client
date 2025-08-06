/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"crypto/ecdsa"
	"crypto/x509"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	hash2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/hash"
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
		return hash2.Hashable(d).Raw()
	}
	panic("unsupported key")
}

func ecdsaPubKeyID(key *ecdsa.PublicKey) ([]byte, error) {
	marshaledPubKey, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal PK")
	}

	h, err := hash2.SHA256(marshaledPubKey)
	if err != nil {
		return nil, errors.Errorf("hash failure")
	}
	return []byte(base58.Encode(h)), nil
}
