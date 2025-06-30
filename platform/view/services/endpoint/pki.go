/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"crypto/x509"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
)

type DefaultPublicKeyIDSynthesizer struct{}

func (d DefaultPublicKeyIDSynthesizer) PublicKeyID(key any) []byte {
	raw, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil
	}
	return hash.Hashable(raw).Raw()
}
