/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// FuzzPKExtractorExtractPublicKey fuzzes PKExtractor.ExtractPublicKey with arbitrary
// PEM/certificate byte payloads. ExtractPublicKey is only ever called on identity bytes
// endpoint.Service already treats as local and trusted: resolver entries loaded from
// core.yaml and the node's own certificate (see websocket/provider.go). It is not on a
// path that receives a remote peer's raw bytes, but its PEM/X.509 parsing is shared code
// worth fuzzing for panics regardless of where the bytes originate.
func FuzzPKExtractorExtractPublicKey(f *testing.F) {
	// 1. Valid ECDSA certificate PEM
	validCertPEM := generateTestCert(f)
	f.Add(validCertPEM)

	// 2. Valid RSA certificate PEM
	f.Add(generateTestRSACert(f))

	// 3. Non-certificate PEM blocks
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("some public key data")})
	f.Add(pubKeyPEM)
	privKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("some private key data")})
	f.Add(privKeyPEM)

	// 4. PEM block with invalid DER bytes
	corruptedCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("corrupted certificate der")})
	f.Add(corruptedCert)

	// 5. Raw non-PEM bytes and boundary values
	f.Add([]byte(nil))
	f.Add([]byte(""))
	f.Add([]byte("not a pem certificate"))
	f.Add([]byte("-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----"))
	f.Add([]byte("-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----"))

	extractor := &PKExtractor{}

	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = extractor.ExtractPublicKey(data)
	})
}

func generateTestRSACert(tb testing.TB) []byte {
	tb.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(tb, err)

	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "rsa-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour * 24),
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	require.NoError(tb, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
}
