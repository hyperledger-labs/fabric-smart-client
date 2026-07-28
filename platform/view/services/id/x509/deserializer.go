/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Deserializer struct{}

// DeserializeVerifier extracts an ECDSA public key from raw and returns a
// Verifier for it. It does NOT validate the key against any MSP or CA: any
// caller-supplied PEM-encoded ECDSA public key deserializes successfully,
// including one belonging to a self-signed or entirely fabricated identity.
// This deserializer is registered unconditionally into the multiplex
// deserializer used by sig.Service, so its output must not be treated as an
// authorization decision on its own - callers must check the resulting
// identity against an explicit allow-list before trusting it.
func (x *Deserializer) DeserializeVerifier(raw []byte) (driver.Verifier, error) {
	genericPublicKey, err := PemDecodeKey(raw)
	if err != nil {
		return nil, errors.Wrap(err, "failed parsing received public key")
	}
	publicKey, ok := genericPublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("expected *ecdsa.PublicKey")
	}

	return NewVerifier(publicKey), nil
}

func (x *Deserializer) DeserializeSigner(raw []byte) (driver.Signer, error) {
	return nil, errors.New("not supported")
}

func (x *Deserializer) Info(raw, auditInfo []byte) (string, error) {
	cert, err := PemDecodeCert(raw)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("X509: [%s][%s]", view.Identity(raw).UniqueID(), cert.Subject.CommonName), nil
}
