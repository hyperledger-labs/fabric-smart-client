/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPemDecodeCert(t *testing.T) {
	t.Parallel()
	_, certPEM := generateSelfSignedCert(t)
	cert, err := PemDecodeCert(certPEM)
	require.NoError(t, err)
	require.Equal(t, "test-user", cert.Subject.CommonName)
}

func TestPemDecodeCert_NotPEM(t *testing.T) {
	t.Parallel()
	_, err := PemDecodeCert([]byte("garbage"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not PEM encoded")
}

func TestPemDecodeCert_BadType(t *testing.T) {
	t.Parallel()
	block := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("data")})
	_, err := PemDecodeCert(block)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad type")
}

func TestPemDecodeCert_InvalidDER(t *testing.T) {
	t.Parallel()
	badCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad der data")})
	_, err := PemDecodeCert(badCert)
	require.Error(t, err)
	require.Contains(t, err.Error(), "pem bytes are not cert encoded")
}
