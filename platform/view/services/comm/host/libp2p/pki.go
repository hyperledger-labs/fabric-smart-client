/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

type PKISupport struct{}

func (p PKISupport) ExtractPublicKey(id view.Identity) (any, error) {
	certRaw, _ := pem.Decode(id)
	if certRaw == nil {
		return nil, errors.Errorf("pem decoding returned nil")
	}

	cert, err := x509.ParseCertificate(certRaw.Bytes)
	if err != nil {
		return nil, err
	}
	raw, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return nil, err
	}
	pk, err := x509.ParsePKIXPublicKey(raw)
	if err != nil {
		return nil, err
	}
	return pk, nil
}

func (p PKISupport) PublicKeyID(key any) []byte {
	switch d := key.(type) {
	case *ecdsa.PublicKey:
		raw, err := x509.MarshalPKIXPublicKey(d)
		if err != nil {
			return nil
		}
		pk, err := crypto.UnmarshalECDSAPublicKey(raw)
		if err != nil {
			return nil
		}
		ID, err := peer.IDFromPublicKey(pk)
		if err != nil {
			return nil
		}
		return []byte(ID.String())
	case []byte:
		return hash.Hashable(d).Raw()
	}
	panic("unsupported key")
}
