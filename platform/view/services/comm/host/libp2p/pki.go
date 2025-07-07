/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"crypto/ecdsa"
	"crypto/x509"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/hash"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

type PKIDSynthesizer struct{}

func (p PKIDSynthesizer) PublicKeyID(key any) []byte {
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
