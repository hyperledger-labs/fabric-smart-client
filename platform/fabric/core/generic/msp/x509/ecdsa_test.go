/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSigner(t *testing.T) {
	t.Parallel()
	id, signer, verifier, err := NewSigner()
	require.NoError(t, err)
	require.NotNil(t, id)
	require.NotNil(t, signer)
	require.NotNil(t, verifier)

	msg := []byte("hello world")
	sig, err := signer.Sign(msg)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(msg, sig))
	require.Error(t, verifier.Verify([]byte("tampered"), sig))
}

func TestPemEncodeDecodeKey(t *testing.T) {
	t.Parallel()
	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// public key round-trip
	pubPEM, err := PemEncodeKey(sk.Public())
	require.NoError(t, err)
	decoded, err := PemDecodeKey(pubPEM)
	require.NoError(t, err)
	require.IsType(t, &ecdsa.PublicKey{}, decoded)

	// private key round-trip
	privPEM, err := PemEncodeKey(sk)
	require.NoError(t, err)
	decoded, err = PemDecodeKey(privPEM)
	require.NoError(t, err)
	require.IsType(t, &ecdsa.PrivateKey{}, decoded)
}

func TestPemDecodeKey_Errors(t *testing.T) {
	t.Parallel()
	_, err := PemDecodeKey([]byte("not pem"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not PEM encoded")

	badBlock := pem.EncodeToMemory(&pem.Block{Type: "UNKNOWN", Bytes: []byte("bad")})
	_, err = PemDecodeKey(badBlock)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad key type")
}

func TestPemDecodeKey_Certificate(t *testing.T) {
	t.Parallel()
	_, certPEM := generateSelfSignedCert(t)
	key, err := PemDecodeKey(certPEM)
	require.NoError(t, err)
	require.IsType(t, &ecdsa.PublicKey{}, key)
}

func TestNewIdentityFromBytes(t *testing.T) {
	t.Parallel()
	id, signer, _, err := NewSigner()
	require.NoError(t, err)

	identity, verifier, err := NewIdentityFromBytes(id)
	require.NoError(t, err)
	require.NotNil(t, identity)

	msg := []byte("test message")
	sig, err := signer.Sign(msg)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(msg, sig))
}

func TestNewIdentityFromBytes_Errors(t *testing.T) {
	t.Parallel()
	_, _, err := NewIdentityFromBytes([]byte("garbage"))
	require.Error(t, err)
}

func TestIsLowS(t *testing.T) {
	t.Parallel()
	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	halfOrder := new(big.Int).Rsh(sk.PublicKey.Curve.Params().N, 1)

	low, err := IsLowS(&sk.PublicKey, big.NewInt(1))
	require.NoError(t, err)
	require.True(t, low)

	high := new(big.Int).Add(halfOrder, big.NewInt(1))
	low, err = IsLowS(&sk.PublicKey, high)
	require.NoError(t, err)
	require.False(t, low)
}

func TestToLowS(t *testing.T) {
	t.Parallel()
	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	halfOrder := new(big.Int).Rsh(sk.PublicKey.Curve.Params().N, 1)
	high := new(big.Int).Add(halfOrder, big.NewInt(1))

	result, modified, err := ToLowS(&sk.PublicKey, high)
	require.NoError(t, err)
	require.True(t, modified)

	low, err := IsLowS(&sk.PublicKey, result)
	require.NoError(t, err)
	require.True(t, low)
}

func TestPemEncodeKey_UnsupportedType(t *testing.T) {
	t.Parallel()
	_, err := PemEncodeKey("not a key")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected key type")
}

func TestVerifier_InvalidSignature(t *testing.T) {
	t.Parallel()
	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	v := NewVerifier(&sk.PublicKey)
	err = v.Verify([]byte("msg"), []byte("not asn1"))
	require.Error(t, err)
}

func TestVerifier_WrongMessage(t *testing.T) {
	t.Parallel()
	id, signer, _, err := NewSigner()
	require.NoError(t, err)

	_, verifier, err := NewIdentityFromBytes(id)
	require.NoError(t, err)

	sig, err := signer.Sign([]byte("original"))
	require.NoError(t, err)
	err = verifier.Verify([]byte("different"), sig)
	require.Error(t, err)
	require.Contains(t, err.Error(), "signature not valid")
}

func TestPemDecodeKey_InvalidPrivateKey(t *testing.T) {
	t.Parallel()
	badKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("bad data")})
	_, err := PemDecodeKey(badKey)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PKCS8 encoded")
}

func TestPemDecodeKey_InvalidPublicKey(t *testing.T) {
	t.Parallel()
	badKey := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("bad data")})
	_, err := PemDecodeKey(badKey)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PKIX encoded")
}

func TestPemEncodeKey_RSAPrivate(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privPEM, err := PemEncodeKey(rsaKey)
	require.NoError(t, err)
	require.NotEmpty(t, privPEM)
	require.Contains(t, string(privPEM), "PRIVATE KEY")
}

func TestPemEncodeKey_RSAPublic(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pubPEM, err := PemEncodeKey(&rsaKey.PublicKey)
	require.NoError(t, err)
	require.NotEmpty(t, pubPEM)
	require.Contains(t, string(pubPEM), "PUBLIC KEY")
}

func TestNewIdentityFromBytes_NonECDSA(t *testing.T) {
	t.Parallel()
	// Use a private key PEM which will decode but won't be *ecdsa.PublicKey
	badKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("bad")})
	si := serializeIdentity(t, "msp1", badKey)

	_, _, err := NewIdentityFromBytes(si)
	require.Error(t, err)
}
