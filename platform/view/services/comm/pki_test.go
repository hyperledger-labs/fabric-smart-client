/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func generateTestCert(t *testing.T) []byte {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
}

func TestExtractPublicKey(t *testing.T) {
	t.Parallel()

	extractor := &PKExtractor{}

	t.Run("Valid Certificate", func(t *testing.T) {
		t.Parallel()
		certPEM := generateTestCert(t)

		pk, err := extractor.ExtractPublicKey(certPEM)
		require.NoError(t, err)
		require.NotNil(t, pk)
		_, ok := pk.(*ecdsa.PublicKey)
		require.True(t, ok)
	})

	t.Run("Invalid PEM", func(t *testing.T) {
		t.Parallel()
		_, err := extractor.ExtractPublicKey([]byte("invalid pem data"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "pem decoding returned nil")
	})

	t.Run("Invalid Certificate Bytes", func(t *testing.T) {
		t.Parallel()
		// Valid PEM block, but invalid certificate data (base64 for "abcd")
		invalidCertPEM := []byte("-----BEGIN CERTIFICATE-----\nYWJjZA==\n-----END CERTIFICATE-----")
		_, err := extractor.ExtractPublicKey(invalidCertPEM)
		require.Error(t, err)
	})
}
