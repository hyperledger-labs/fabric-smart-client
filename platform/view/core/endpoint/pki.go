/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"crypto/x509"
	"encoding/pem"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type PkiResolver struct {
}

func NewPKIResolver() *PkiResolver {
	return &PkiResolver{}
}

func (p PkiResolver) GetPKIidOfCert(peerIdentity view.Identity) []byte {
	certRaw, _ := pem.Decode(peerIdentity)
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
		pubclikey, err := crypto.UnmarshalECDSAPublicKey(raw)
		if err != nil {
			return nil
		}
		ID, err := peer.IDFromPublicKey(pubclikey)
		if err != nil {
			return nil
		}

		return []byte(ID.String())
	default:
		return nil
	}
}
