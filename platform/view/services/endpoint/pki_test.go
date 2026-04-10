/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package endpoint_test contains tests for the endpoint service package,
// including tests for PKI operations, service functionality, and resolver management.
package endpoint_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultPublicKeyIDSynthesizer_PublicKeyID(t *testing.T) {
	t.Parallel()
	synthesizer := endpoint.DefaultPublicKeyIDSynthesizer{}

	t.Run("RSA public key", func(t *testing.T) {
		t.Parallel()
		// Generate RSA key
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		// Get public key ID
		pkID, err := synthesizer.PublicKeyID(&privateKey.PublicKey)
		require.NoError(t, err)
		require.NotNil(t, pkID)
		require.Len(t, pkID, 32) // SHA256 produces 32 bytes

		// Verify it's deterministic
		pkID2, err := synthesizer.PublicKeyID(&privateKey.PublicKey)
		require.NoError(t, err)
		assert.Equal(t, pkID, pkID2)

		// Verify it's actually the hash of the marshaled key
		raw, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
		require.NoError(t, err)
		expectedHash := sha256.Sum256(raw)
		assert.Equal(t, expectedHash[:], pkID)
	})

	t.Run("ECDSA public key", func(t *testing.T) {
		t.Parallel()
		// Generate ECDSA key
		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		// Get public key ID
		pkID, err := synthesizer.PublicKeyID(&privateKey.PublicKey)
		require.NoError(t, err)
		require.NotNil(t, pkID)
		require.Len(t, pkID, 32)

		// Verify it's deterministic
		pkID2, err := synthesizer.PublicKeyID(&privateKey.PublicKey)
		require.NoError(t, err)
		assert.Equal(t, pkID, pkID2)
	})

	t.Run("different keys produce different IDs", func(t *testing.T) {
		t.Parallel()
		// Generate two different keys
		key1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		key2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		pkID1, err := synthesizer.PublicKeyID(&key1.PublicKey)
		require.NoError(t, err)
		pkID2, err := synthesizer.PublicKeyID(&key2.PublicKey)
		require.NoError(t, err)

		assert.NotEqual(t, pkID1, pkID2)
	})

	t.Run("unsupported key type", func(t *testing.T) {
		t.Parallel()
		// Test with unsupported key type
		_, err := synthesizer.PublicKeyID("not a key")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "x509:")
	})

	t.Run("nil key", func(t *testing.T) {
		t.Parallel()
		_, err := synthesizer.PublicKeyID(nil)
		require.Error(t, err)
	})

	t.Run("byte slice key (should fail)", func(t *testing.T) {
		t.Parallel()
		// This should fail as x509.MarshalPKIXPublicKey doesn't support []byte
		_, err := synthesizer.PublicKeyID([]byte("some bytes"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported public key type")
	})
}
