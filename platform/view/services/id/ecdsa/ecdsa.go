/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ecdsa

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"math/big"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ecdsaSignature struct {
	R, S *big.Int
}

type dsaSigner struct {
	sk *ecdsa.PrivateKey
}

func (d dsaSigner) Sign(message []byte) ([]byte, error) {
	hash := sha256.New()
	n, err := hash.Write(message)
	if n != len(message) {
		return nil, errors.Errorf("hash failure")
	}
	if err != nil {
		return nil, err
	}
	digest := hash.Sum(nil)

	return d.sk.Sign(rand.Reader, digest, nil)
}

type dsaVerifier struct {
	pk *ecdsa.PublicKey
}

func (d dsaVerifier) Verify(message, sigma []byte) error {
	signature := &ecdsaSignature{}
	_, err := asn1.Unmarshal(sigma, signature)
	if err != nil {
		return err
	}

	hash := sha256.New()
	n, err := hash.Write(message)
	if n != len(message) {
		return errors.Errorf("hash failure")
	}
	if err != nil {
		return err
	}
	digest := hash.Sum(nil)

	valid := ecdsa.Verify(d.pk, digest, signature.R, signature.S)
	if !valid {
		return errors.Errorf("signature not valid")
	}

	return nil
}

func NewSigner() (view.Identity, driver.Signer, driver.Verifier, error) {
	// Create ephemeral key and store it in the context
	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	pkRaw, err := x509.MarshalPKIXPublicKey(sk.Public())
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed marshalling public key")
	}

	return pkRaw, &dsaSigner{sk: sk}, &dsaVerifier{pk: &sk.PublicKey}, nil
}

func NewSignerFromPEM(raw []byte) (driver.Signer, error) {
	p, _ := pem.Decode(raw)
	// Create ephemeral key and store it in the context
	sk, err := x509.ParsePKCS8PrivateKey(p.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling secret key")
	}

	return &dsaSigner{sk: sk.(*ecdsa.PrivateKey)}, nil
}

func NewIdentityFromBytes(raw []byte) (view.Identity, driver.Verifier, error) {
	genericPublicKey, err := x509.ParsePKIXPublicKey(raw)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed parsing received public key")
	}
	publicKey, ok := genericPublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, nil, errors.New("expected *ecdsa.PublicKey")
	}

	return raw, &dsaVerifier{pk: publicKey}, nil
}

func NewIdentityFromPEMCert(raw []byte) (view.Identity, driver.Verifier, error) {
	p, _ := pem.Decode(raw)
	cert, err := x509.ParseCertificate(p.Bytes)
	if err != nil {
		return nil, nil, err
	}
	publicKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, nil, errors.New("expected *ecdsa.PublicKey")
	}

	return raw, &dsaVerifier{pk: publicKey}, nil
}
