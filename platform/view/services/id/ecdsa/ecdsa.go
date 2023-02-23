/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ecdsa

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"math/big"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

// curveHalfOrders contains the precomputed curve group orders halved.
// It is used to ensure that signature' S value is lower or equal to the
// curve group order halved. We accept only low-S signatures.
// They are precomputed for efficiency reasons.
var curveHalfOrders = map[elliptic.Curve]*big.Int{
	elliptic.P224(): new(big.Int).Rsh(elliptic.P224().Params().N, 1),
	elliptic.P256(): new(big.Int).Rsh(elliptic.P256().Params().N, 1),
	elliptic.P384(): new(big.Int).Rsh(elliptic.P384().Params().N, 1),
	elliptic.P521(): new(big.Int).Rsh(elliptic.P521().Params().N, 1),
}

func GetCurveHalfOrdersAt(c elliptic.Curve) *big.Int {
	return big.NewInt(0).Set(curveHalfOrders[c])
}

type Signature struct {
	R, S *big.Int
}

// Signer implements the crypto.Signer interface for ECDSA keys.  The
// Sign method ensures signatures are created with Low S values since Fabric
// normalizes all signatures to Low S.
// See https://github.com/bitcoin/bips/blob/master/bip-0146.mediawiki#low_s
// for more detail.
type Signer struct {
	PrivateKey *ecdsa.PrivateKey
}

// Public returns the ecdsa.PublicKey associated with PrivateKey.
func (e *Signer) Public() crypto.PublicKey {
	return &e.PrivateKey.PublicKey
}

// Sign signs the digest and ensures that signatures use the Low S value.
func (e *Signer) Sign(message []byte) ([]byte, error) {
	hash := sha256.New()
	n, err := hash.Write(message)
	if err != nil {
		return nil, errors.Wrap(err, "failed to write message to hash")
	}
	if n != len(message) {
		return nil, errors.Errorf("hash failure")
	}
	digest := hash.Sum(nil)

	r, s, err := ecdsa.Sign(rand.Reader, e.PrivateKey, digest)
	if err != nil {
		return nil, err
	}

	// ensure Low S signatures
	sig := toLowS(
		e.PrivateKey.PublicKey,
		Signature{
			R: r,
			S: s,
		},
	)

	// return marshaled signature
	return asn1.Marshal(sig)
}

type Verifier struct {
	pk *ecdsa.PublicKey
}

func (d Verifier) Verify(message, sigma []byte) error {
	signature := &Signature{}
	_, err := asn1.Unmarshal(sigma, signature)
	if err != nil {
		return errors.Wrap(err, "failed unmarshalling signature")
	}

	lowS, err := IsLowS(d.pk, signature.S)
	if err != nil {
		return err
	}

	if !lowS {
		return errors.Errorf("invalid s, must be smaller than half the order [%s][%s]", signature.S, GetCurveHalfOrdersAt(d.pk.Curve))
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

	return pkRaw, &Signer{PrivateKey: sk}, &Verifier{pk: &sk.PublicKey}, nil
}

func NewSignerFromPEM(raw []byte) (driver.Signer, error) {
	p, _ := pem.Decode(raw)
	// Create ephemeral key and store it in the context
	sk, err := x509.ParsePKCS8PrivateKey(p.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling secret key")
	}

	return &Signer{PrivateKey: sk.(*ecdsa.PrivateKey)}, nil
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

	return raw, &Verifier{pk: publicKey}, nil
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

	return raw, &Verifier{pk: publicKey}, nil
}

/*
*
When using ECDSA, both (r,s) and (r, -s mod n) are valid signatures.  In order
to protect against signature malleability attacks, Fabric normalizes all
signatures to a canonical form where s is at most half the order of the curve.
In order to make signatures compliant with what Fabric expects, toLowS creates
signatures in this canonical form.
*/
func toLowS(key ecdsa.PublicKey, sig Signature) Signature {
	// calculate half order of the curve
	halfOrder := new(big.Int).Div(key.Curve.Params().N, big.NewInt(2))
	// check if s is greater than half order of curve
	if sig.S.Cmp(halfOrder) == 1 {
		// Set s to N - s so that s will be less than or equal to half order
		sig.S.Sub(key.Params().N, sig.S)
	}
	return sig
}

// IsLowS checks that s is a low-S
func IsLowS(k *ecdsa.PublicKey, s *big.Int) (bool, error) {
	halfOrder, ok := curveHalfOrders[k.Curve]
	if !ok {
		return false, fmt.Errorf("curve not recognized [%s]", k.Curve)
	}

	return s.Cmp(halfOrder) != 1, nil
}
