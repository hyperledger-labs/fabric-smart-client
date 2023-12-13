/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

type PKISupport struct{}

func (p PKISupport) ExtractPublicKey(id view.Identity) any {
	certRaw, _ := pem.Decode(id)
	switch {
	case certRaw != nil:
		cert, err := x509.ParseCertificate(certRaw.Bytes)
		if err != nil {
			return nil
		}
		raw, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
		if err != nil {
			return nil
		}
		pk, err := x509.ParsePKIXPublicKey(raw)
		if err != nil {
			return nil
		}
		return pk
	default:
		return nil
	}
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
