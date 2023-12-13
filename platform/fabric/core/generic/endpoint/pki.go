/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-protos-go/msp"
)

type PublicKeyExtractor struct{}

func (p PublicKeyExtractor) ExtractPublicKey(id view.Identity) (any, error) {
	si := &msp.SerializedIdentity{}
	err := proto.Unmarshal(id, si)
	if err != nil {
		return nil, err
	}

	certRaw, _ := pem.Decode(si.IdBytes)
	switch {
	case certRaw != nil:
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
	default:
		// This can only be an idemix identity then
		serialized := &msp.SerializedIdemixIdentity{}
		err := proto.Unmarshal(si.IdBytes, serialized)
		if err != nil {
			return nil, err
		}

		h := sha256.New()
		h.Write(serialized.NymX)
		h.Write(serialized.NymY)
		h.Write(serialized.Proof)
		h.Write(serialized.Ou)
		h.Write(serialized.Role)
		return h.Sum(nil), nil
	}
}
