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

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestPemDecodeCert(t *testing.T) {
	t.Parallel()

	_, certPEM, _, _ := newTestMaterial(t, "alice")

	cert, err := PemDecodeCert(certPEM)
	require.NoError(t, err)
	require.Equal(t, "alice", cert.Subject.CommonName)

	_, err = PemDecodeCert([]byte("not pem"))
	require.EqualError(t, err, "bytes are not PEM encoded")

	wrongType := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("abc")})
	_, err = PemDecodeCert(wrongType)
	require.EqualError(t, err, "bad type PUBLIC KEY, expected 'CERTIFICATE")
}

func TestPemDecodeKey(t *testing.T) {
	t.Parallel()

	privateKey, certPEM, publicKeyPEM, privateKeyPEM := newTestMaterial(t, "bob")

	key, err := PemDecodeKey(privateKeyPEM)
	require.NoError(t, err)
	privateKeyOut, ok := key.(*ecdsa.PrivateKey)
	require.True(t, ok)
	require.Equal(t, privateKey.D, privateKeyOut.D)

	key, err = PemDecodeKey(publicKeyPEM)
	require.NoError(t, err)
	publicKeyOut, ok := key.(*ecdsa.PublicKey)
	require.True(t, ok)
	require.Equal(t, privateKey.X, publicKeyOut.X)
	require.Equal(t, privateKey.Y, publicKeyOut.Y)
	testMessage := []byte("test message for verification")
	signature, err := signMessage(t, privateKey, testMessage)
	require.NoError(t, err)
	verifier := NewVerifier(publicKeyOut)
	require.NoError(t, verifier.Verify(testMessage, signature), "public key extracted from PEM should verify signatures")

	key, err = PemDecodeKey(certPEM)
	require.NoError(t, err)
	publicKeyOut, ok = key.(*ecdsa.PublicKey)
	require.True(t, ok)
	require.Equal(t, privateKey.X, publicKeyOut.X)
	require.Equal(t, privateKey.Y, publicKeyOut.Y)
	require.NoError(t, NewVerifier(publicKeyOut).Verify(testMessage, signature), "public key extracted from certificate should verify signatures")

	_, err = PemDecodeKey([]byte("not pem"))
	require.EqualError(t, err, "bytes are not PEM encoded")

	wrongType := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("abc")})
	_, err = PemDecodeKey(wrongType)
	require.EqualError(t, err, "bad key type RSA PRIVATE KEY")
}

func TestVerifierAndDeserializer(t *testing.T) {
	t.Parallel()

	privateKey, certPEM, publicKeyPEM, _ := newTestMaterial(t, "charlie")
	message := []byte("hello world")

	digest := sha256Digest(message)
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, digest)
	require.NoError(t, err)

	lowS, modified, err := ToLowS(&privateKey.PublicKey, s)
	require.NoError(t, err)
	require.False(t, modified && lowS == nil)

	signature, err := MarshalECDSASignature(r, lowS)
	require.NoError(t, err)

	verifier := NewVerifier(&privateKey.PublicKey)
	require.NoError(t, verifier.Verify(message, signature))

	badSignature, err := MarshalECDSASignature(r, new(big.Int).Sub(privateKey.Params().N, lowS))
	require.NoError(t, err)
	require.EqualError(t, verifier.Verify(message, badSignature), "signature is not in lowS")

	deserializer := &Deserializer{}
	decodedVerifier, err := deserializer.DeserializeVerifier(publicKeyPEM)
	require.NoError(t, err)
	require.NoError(t, decodedVerifier.Verify(message, signature))

	info, err := deserializer.Info(certPEM, nil)
	require.NoError(t, err)
	require.Contains(t, info, "X509: [")
	require.Contains(t, info, "][charlie]")
	require.Contains(t, info, view.Identity(certPEM).UniqueID())

	_, err = deserializer.DeserializeVerifier([]byte("not pem"))
	require.ErrorContains(t, err, "failed parsing received public key")

	_, err = deserializer.DeserializeSigner(certPEM)
	require.EqualError(t, err, "not supported")
}

func TestIsLowSAndToLowS(t *testing.T) {
	t.Parallel()

	privateKey, _, _, _ := newTestMaterial(t, "delta")

	halfOrder := new(big.Int).Rsh(privateKey.Params().N, 1)
	isLow, err := IsLowS(&privateKey.PublicKey, new(big.Int).Set(halfOrder))
	require.NoError(t, err)
	require.True(t, isLow)

	highS := new(big.Int).Sub(privateKey.Params().N, big.NewInt(1))
	isLow, err = IsLowS(&privateKey.PublicKey, new(big.Int).Set(highS))
	require.NoError(t, err)
	require.False(t, isLow)

	normalized, modified, err := ToLowS(&privateKey.PublicKey, highS)
	require.NoError(t, err)
	require.True(t, modified)
	require.Equal(t, big.NewInt(1), normalized)

	normalized, modified, err = ToLowS(&privateKey.PublicKey, new(big.Int).Set(halfOrder))
	require.NoError(t, err)
	require.False(t, modified)
	require.Equal(t, halfOrder, normalized)
}

func TestVerifyRejectsInvalidEncodings(t *testing.T) {
	t.Parallel()

	privateKey, _, _, _ := newTestMaterial(t, "echo")
	verifier := NewVerifier(&privateKey.PublicKey)

	err := verifier.Verify([]byte("msg"), []byte("not asn1"))
	require.Error(t, err)

	validLowS, err := signMessage(t, privateKey, []byte("msg"))
	require.NoError(t, err)

	err = verifier.Verify([]byte("different"), validLowS)
	require.EqualError(t, err, "signature not valid")
}

func newTestMaterial(t *testing.T, commonName string) (*ecdsa.PrivateKey, []byte, []byte, []byte) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &stdx509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              stdx509.KeyUsageDigitalSignature | stdx509.KeyUsageCertSign,
		ExtKeyUsage:           []stdx509.ExtKeyUsage{stdx509.ExtKeyUsageClientAuth, stdx509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := stdx509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	privateKeyDER, err := stdx509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	publicKeyDER, err := stdx509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicKeyDER})
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER})

	return privateKey, certPEM, publicKeyPEM, privateKeyPEM
}

func signMessage(t *testing.T, privateKey *ecdsa.PrivateKey, message []byte) ([]byte, error) {
	t.Helper()

	digest := sha256Digest(message)
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, digest)
	if err != nil {
		return nil, err
	}

	lowS, _, err := ToLowS(&privateKey.PublicKey, s)
	if err != nil {
		return nil, err
	}

	signature, err := MarshalECDSASignature(r, lowS)
	if err != nil {
		return nil, err
	}
	return signature, nil
}

func sha256Digest(message []byte) []byte {
	sum := sha256.Sum256(message)
	return sum[:]
}
