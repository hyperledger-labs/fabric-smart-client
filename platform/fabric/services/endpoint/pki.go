/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package endpoint

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// TODO: move under generic
type pkiResolver struct {
}

func NewPKIResolver() *pkiResolver {
	return &pkiResolver{}
}

func (p pkiResolver) GetPKIidOfCert(peerIdentity view.Identity) []byte {
	si := &msp.SerializedIdentity{}
	err := proto.Unmarshal(peerIdentity, si)
	if err != nil {
		return nil
	}

	certRaw, _ := pem.Decode(si.IdBytes)
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
		// This can only be an idemix identity then
		serialized := &msp.SerializedIdemixIdentity{}
		err := proto.Unmarshal(si.IdBytes, serialized)
		if err != nil {
			return nil
		}

		h := sha256.New()
		h.Write(serialized.NymX)
		h.Write(serialized.NymY)
		h.Write(serialized.Proof)
		h.Write(serialized.Ou)
		h.Write(serialized.Role)
		return h.Sum(nil)
	}
}
