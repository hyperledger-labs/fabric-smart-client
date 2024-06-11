/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tss

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"math/big"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp/utils"
)

var (
	// curveHalfOrders contains the precomputed curve group orders halved.
	// It is used to ensure that signature' S value is lower or equal to the
	// curve group order halved. We accept only low-S signatures.
	// They are precomputed for efficiency reasons.
	curveHalfOrders = map[elliptic.Curve]*big.Int{
		elliptic.P224(): new(big.Int).Rsh(elliptic.P224().Params().N, 1),
		elliptic.P256(): new(big.Int).Rsh(elliptic.P256().Params().N, 1),
		elliptic.P384(): new(big.Int).Rsh(elliptic.P384().Params().N, 1),
		elliptic.P521(): new(big.Int).Rsh(elliptic.P521().Params().N, 1),
	}
)

type ecdsaSignature struct {
	R, S *big.Int
}

type ecdsaSigner struct {
	sk *ecdsa.PrivateKey
}

func NewECDSASigner() (view.Identity, driver.Signer, driver.Verifier, error) {
	// Create ephemeral key and store it in the context
	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	pkRaw, err := PemEncodeKey(sk.Public())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed marshalling public key: %w", err)
	}

	mspSI := &msp.SerializedIdentity{
		//Type:    msp.SerializedIdentity_PK,
		IdBytes: pkRaw,
	}
	idRaw, err := proto.Marshal(mspSI)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed marshalling msp serialized identity: %w", err)
	}

	return idRaw, &ecdsaSigner{sk: sk}, &ecdsaVerifier{pk: &sk.PublicKey}, nil
}

func (d *ecdsaSigner) Sign(message []byte) ([]byte, error) {
	dgst := sha256.Sum256(message)

	r, s, err := ecdsa.Sign(rand.Reader, d.sk, dgst[:])
	if err != nil {
		return nil, err
	}

	s, _, err = ToLowS(&d.sk.PublicKey, s)
	if err != nil {
		return nil, err
	}

	return utils.MarshalECDSASignature(r, s)
}

type ecdsaVerifier struct {
	pk *ecdsa.PublicKey
}

func NewECDSAVerifier(pk *ecdsa.PublicKey) *ecdsaVerifier {
	return &ecdsaVerifier{pk: pk}
}

func (d *ecdsaVerifier) Verify(message, sigma []byte) error {
	signature := &ecdsaSignature{}
	_, err := asn1.Unmarshal(sigma, signature)
	if err != nil {
		return err
	}
	lowS, err := IsLowS(d.pk, signature.S)
	if err != nil {
		return err
	}
	if !lowS {
		return fmt.Errorf("signature is not in lowS")
	}

	hash := sha256.Sum256(message)
	valid := ecdsa.VerifyASN1(d.pk, hash[:], sigma)
	if !valid {
		return fmt.Errorf("signature not valid")
	}
	return nil
}

func NewIdentityFromBytes(raw []byte) (view.Identity, driver.Verifier, error) {
	mspSI := &msp.SerializedIdentity{}
	err := proto.Unmarshal(raw, mspSI)
	if err != nil {
		return nil, nil, fmt.Errorf("failed unmarshalling to msp serialized identity: %w", err)
	}

	genericPublicKey, err := PemDecodeKey(mspSI.IdBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed parsing received public key: %w", err)
	}
	publicKey, ok := genericPublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, nil, fmt.Errorf("expected *ecdsa.PublicKey")
	}

	return raw, &ecdsaVerifier{pk: publicKey}, nil
}

// PemEncodeKey takes a Go key and converts it to bytes
func PemEncodeKey(key interface{}) ([]byte, error) {
	var encoded []byte
	var err error
	var keyType string

	switch key.(type) {
	case *ecdsa.PrivateKey, *rsa.PrivateKey:
		keyType = "PRIVATE"
		encoded, err = x509.MarshalPKCS8PrivateKey(key)
	case *ecdsa.PublicKey, *rsa.PublicKey:
		keyType = "PUBLIC"
		encoded, err = x509.MarshalPKIXPublicKey(key)
	default:
		err = fmt.Errorf("programming error, unexpected key type %T", key)
	}
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: keyType + " KEY", Bytes: encoded}), nil
}

// PemDecodeKey takes bytes and returns a Go key
func PemDecodeKey(keyBytes []byte) (interface{}, error) {
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, fmt.Errorf("bytes are not PEM encoded")
	}

	var key interface{}
	var err error
	switch block.Type {
	case "PRIVATE KEY":
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("pem bytes are not PKCS8 encoded: %w", err)
		}
	case "CERTIFICATE":
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("pem bytes are not cert encoded: %w", err)
		}
		return cert.PublicKey, nil
	case "PUBLIC KEY":
		key, err = x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("pem bytes are not PKIX encoded: %w", err)
		}
	default:
		return nil, fmt.Errorf("bad key type %s", block.Type)
	}

	return key, nil
}

// IsLow checks that s is a low-S
func IsLowS(k *ecdsa.PublicKey, s *big.Int) (bool, error) {
	halfOrder, ok := curveHalfOrders[k.Curve]
	if !ok {
		return false, fmt.Errorf("curve not recognized [%s]", k.Curve)
	}

	return s.Cmp(halfOrder) != 1, nil

}

func ToLowS(k *ecdsa.PublicKey, s *big.Int) (*big.Int, bool, error) {
	lowS, err := IsLowS(k, s)
	if err != nil {
		return nil, false, err
	}

	if !lowS {
		// Set s to N - s that will be then in the lower part of signature space
		// less or equal to half order
		s.Sub(k.Params().N, s)

		return s, true, nil
	}

	return s, false, nil
}
