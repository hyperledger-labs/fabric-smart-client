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

type Deserializer struct {
}

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

func (x *Deserializer) Info(raw []byte, auditInfo []byte) (string, error) {
	cert, err := PemDecodeCert(raw)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("X509: [%s][%s]", view.Identity(raw).UniqueID(), cert.Subject.CommonName), nil
}
