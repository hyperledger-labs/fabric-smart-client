/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ecdsa

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetCurveHalfOrdersAt(t *testing.T) {
	t.Parallel()

	halfOrder := GetCurveHalfOrdersAt(elliptic.P256())
	require.NotNil(t, halfOrder)

	// P256 N = FFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551
	// Half should be non-zero and greater than 0
	require.True(t, halfOrder.Cmp(big.NewInt(0)) > 0)
}

func TestNewSigner(t *testing.T) {
	t.Parallel()

	id, signer, verifier, err := NewSigner()
	require.NoError(t, err)
	require.NotNil(t, id)
	require.NotNil(t, signer)
	require.NotNil(t, verifier)

	// Test Public()
	ecdsaSigner := signer.(*Signer)
	pub := ecdsaSigner.Public()
	require.NotNil(t, pub)

	msg := []byte("hello world")
	sig, err := signer.Sign(msg)
	require.NoError(t, err)
	require.NotNil(t, sig)

	err = verifier.Verify(msg, sig)
	require.NoError(t, err)
}

func TestNewSignerFromPEM(t *testing.T) {
	t.Parallel()

	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	skBytes, err := x509.MarshalPKCS8PrivateKey(sk)
	require.NoError(t, err)

	skPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: skBytes,
	})

	signer, err := NewSignerFromPEM(skPEM)
	require.NoError(t, err)
	require.NotNil(t, signer)

	msg := []byte("test message")
	sig, err := signer.Sign(msg)
	require.NoError(t, err)
	require.NotNil(t, sig)

	// Verify using the public key directly
	verifier := &Verifier{pk: &sk.PublicKey}
	err = verifier.Verify(msg, sig)
	require.NoError(t, err)
}

func TestNewSignerFromPEM_Errors(t *testing.T) {
	t.Parallel()

	// Invalid PEM
	_, err := NewSignerFromPEM([]byte("invalid pem"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot pem decode")

	// Valid PEM but RSA key (not ECDSA)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rsaBytes, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	require.NoError(t, err)
	rsaPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: rsaBytes,
	})
	_, err = NewSignerFromPEM(rsaPEM)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected *ecdsa.PrivateKey")
}

func TestNewIdentityFromBytes(t *testing.T) {
	t.Parallel()

	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	pkBytes, err := x509.MarshalPKIXPublicKey(&sk.PublicKey)
	require.NoError(t, err)

	id, verifier, err := NewIdentityFromBytes(pkBytes)
	require.NoError(t, err)
	require.NotNil(t, id)
	require.NotNil(t, verifier)
	require.Equal(t, pkBytes, []byte(id))
}

func TestNewIdentityFromBytes_Errors(t *testing.T) {
	t.Parallel()

	_, _, err := NewIdentityFromBytes([]byte("invalid public key"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed parsing received public key")

	// Valid PKIX but not ECDSA
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rsaPubBytes, err := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
	require.NoError(t, err)

	_, _, err = NewIdentityFromBytes(rsaPubBytes)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected *ecdsa.PublicKey")
}

func TestNewIdentityFromPEMCert(t *testing.T) {
	t.Parallel()

	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour),
		IsCA:      false,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &sk.PublicKey, sk)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	id, verifier, err := NewIdentityFromPEMCert(certPEM)
	require.NoError(t, err)
	require.NotNil(t, id)
	require.NotNil(t, verifier)
}

func TestNewIdentityFromPEMCert_Errors(t *testing.T) {
	t.Parallel()

	_, _, err := NewIdentityFromPEMCert([]byte("invalid pem"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot pem decode")

	// Valid PEM but invalid certificate bytes
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("not a certificate"),
	})
	_, _, err = NewIdentityFromPEMCert(certPEM)
	require.Error(t, err)

	// Valid certificate but not ECDSA
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour),
		IsCA:      false,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &rsaKey.PublicKey, rsaKey)
	require.NoError(t, err)
	rsaCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	_, _, err = NewIdentityFromPEMCert(rsaCertPEM)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected *ecdsa.PublicKey")
}

func TestIsLowS(t *testing.T) {
	t.Parallel()

	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	halfOrder := GetCurveHalfOrdersAt(sk.Curve)

	// S is exactly half order -> valid (low S)
	sLow := new(big.Int).Set(halfOrder)
	isLow, err := IsLowS(&sk.PublicKey, sLow)
	require.NoError(t, err)
	require.True(t, isLow)

	// S is half order + 1 -> invalid (high S)
	sHigh := new(big.Int).Add(halfOrder, big.NewInt(1))
	isLow, err = IsLowS(&sk.PublicKey, sHigh)
	require.NoError(t, err)
	require.False(t, isLow)
}

func TestIsLowS_UnknownCurve(t *testing.T) {
	t.Parallel()

	pk := &ecdsa.PublicKey{}
	_, err := IsLowS(pk, big.NewInt(1))
	require.Error(t, err)
	require.Contains(t, err.Error(), "curve not recognized")
}

func TestToLowS(t *testing.T) {
	t.Parallel()

	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	halfOrder := GetCurveHalfOrdersAt(sk.Curve)
	sHigh := new(big.Int).Add(halfOrder, big.NewInt(1))
	expectedS := new(big.Int).Sub(sk.Curve.Params().N, sHigh)

	sig := Signature{
		R: big.NewInt(1),
		S: sHigh,
	}

	lowSig := toLowS(sk.PublicKey, sig)

	// Must be updated to a value <= halfOrder
	require.True(t, lowSig.S.Cmp(halfOrder) <= 0)
	require.Equal(t, expectedS, lowSig.S)
}

func TestVerifierVerify_Errors(t *testing.T) {
	t.Parallel()

	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	verifier := &Verifier{pk: &sk.PublicKey}

	msg := []byte("hello")

	// Invalid ASN.1 signature
	err = verifier.Verify(msg, []byte("invalid asn1"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed unmarshalling signature")

	// High S signature
	halfOrder := GetCurveHalfOrdersAt(sk.Curve)
	sHigh := new(big.Int).Add(halfOrder, big.NewInt(1))
	highSig := Signature{
		R: big.NewInt(1),
		S: sHigh,
	}
	highSigBytes, _ := asn1.Marshal(highSig)
	err = verifier.Verify(msg, highSigBytes)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid s, must be smaller than half the order")

	// Low S, but invalid signature values (R, S not matching the message)
	lowSig := Signature{
		R: big.NewInt(1),
		S: big.NewInt(1),
	}
	lowSigBytes, _ := asn1.Marshal(lowSig)
	err = verifier.Verify(msg, lowSigBytes)
	require.Error(t, err)
	require.Contains(t, err.Error(), "signature not valid")
}
