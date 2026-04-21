/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

type PKIDSynthesizer struct{}

func (p PKIDSynthesizer) PublicKeyID(key any) ([]byte, error) {
	switch d := key.(type) {
	case *ecdsa.PublicKey:
		raw, err := x509.MarshalPKIXPublicKey(d)
		if err != nil {
			return nil, err
		}
		pk, err := crypto.UnmarshalECDSAPublicKey(raw)
		if err != nil {
			return nil, err
		}
		ID, err := peer.IDFromPublicKey(pk)
		if err != nil {
			return nil, err
		}
		return []byte(ID.String()), nil
	case []byte:
		h := sha256.Sum256(d)
		return h[:], nil
	}

	return nil, errors.Errorf("unsupported key type [%T]", key)
}
