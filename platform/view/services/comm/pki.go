/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"crypto/x509"
	"encoding/pem"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type PKExtractor struct{}

func (p *PKExtractor) ExtractPublicKey(id view.Identity) (any, error) {
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
