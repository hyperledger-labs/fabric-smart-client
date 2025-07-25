/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/client/si"
	"github.com/pkg/errors"
)

func NewX509SigningIdentity(certPath, skPath string) (SigningIdentity, error) {
	cert, err := os.ReadFile(certPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed reading certificate from [%s]", certPath)
	}
	sk, err := os.ReadFile(skPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed reading secret key from [%s]", skPath)
	}
	block, _ := pem.Decode(sk)
	if block == nil {
		return nil, errors.Errorf("failed decoding PEM. Block must be different from nil. [% x]", sk)
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrapf(err, "failed parsing PKCS8 private key from [%v]", block.Bytes)
	}

	switch key := k.(type) {
	case *ecdsa.PrivateKey:
		return newSigningIdentity(cert, si.NewEcdsaSigner(key)), nil
	default:
		return nil, errors.Errorf("key type [%T] not recognized", key)
	}
}

type Signer interface {
	Sign(msg []byte) (signature []byte, err error)
}

type signingIdentity struct {
	serialized []byte
	signer     Signer
}

func newSigningIdentity(serialized []byte, signer Signer) *signingIdentity {
	return &signingIdentity{
		serialized: serialized,
		signer:     signer,
	}
}

func (s *signingIdentity) Serialize() ([]byte, error) {
	return s.serialized, nil
}

func (s *signingIdentity) Sign(message []byte) ([]byte, error) {
	return s.signer.Sign(message)
}
