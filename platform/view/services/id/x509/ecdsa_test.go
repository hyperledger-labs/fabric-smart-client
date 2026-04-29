/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	stdx509 "crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPemDecodeKey_PublicKeyPEMVerifierRoundTrip(t *testing.T) {
	t.Parallel()

	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	pubPEM := pemEncodePublicKey(t, &sk.PublicKey)
	sig := signMessage(t, sk, []byte("hello world"))

	decoded, err := PemDecodeKey(pubPEM)
	require.NoError(t, err)

	pk, ok := decoded.(*ecdsa.PublicKey)
	require.True(t, ok)
	require.NoError(t, NewVerifier(pk).Verify([]byte("hello world"), sig))
}

func TestPemDecodeKey_CertificateVerifierRoundTrip(t *testing.T) {
	t.Parallel()

	sk, certPEM := generateSelfSignedCert(t)
	sig := signMessage(t, sk, []byte("hello world"))

	decoded, err := PemDecodeKey(certPEM)
	require.NoError(t, err)

	pk, ok := decoded.(*ecdsa.PublicKey)
	require.True(t, ok)
	require.NoError(t, NewVerifier(pk).Verify([]byte("hello world"), sig))
}

func TestDeserializer_DeserializeVerifier_VerifiesSignatureFromCertificate(t *testing.T) {
	t.Parallel()

	sk, certPEM := generateSelfSignedCert(t)
	sig := signMessage(t, sk, []byte("payload"))

	deserializer := &Deserializer{}
	verifier, err := deserializer.DeserializeVerifier(certPEM)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify([]byte("payload"), sig))
}

func generateSelfSignedCert(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()

	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &stdx509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-user"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     stdx509.KeyUsageDigitalSignature,
	}

	derBytes, err := stdx509.CreateCertificate(rand.Reader, template, template, &sk.PublicKey, sk)
	require.NoError(t, err)

	return sk, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
}

func pemEncodePublicKey(t *testing.T, pk *ecdsa.PublicKey) []byte {
	t.Helper()

	derBytes, err := stdx509.MarshalPKIXPublicKey(pk)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: derBytes})
}

func signMessage(t *testing.T, sk *ecdsa.PrivateKey, msg []byte) []byte {
	t.Helper()

	digest := sha256.Sum256(msg)
	r, s, err := ecdsa.Sign(rand.Reader, sk, digest[:])
	require.NoError(t, err)

	s, _, err = ToLowS(&sk.PublicKey, s)
	require.NoError(t, err)

	sig, err := MarshalECDSASignature(r, s)
	require.NoError(t, err)

	return sig
}
