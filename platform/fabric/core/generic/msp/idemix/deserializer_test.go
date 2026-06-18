/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewDeserializer(t *testing.T) { //nolint:paralleltest

	// Empty IPK skips issuerPublicKey import
	deserializer, err := NewDeserializer(nil)
	require.NoError(t, err)
	require.NotNil(t, deserializer)

	// Test String()
	require.Contains(t, deserializer.String(), "Idemix with IPK")

	// Test DeserializeSigner (returns not supported error)
	signer, err := deserializer.DeserializeSigner(nil)
	require.ErrorContains(t, err, "not supported")
	require.Nil(t, signer)

	// Test DeserializeVerifier with invalid input
	verifier, err := deserializer.DeserializeVerifier(nil)
	require.Error(t, err)
	require.Nil(t, verifier)

	// Test DeserializeVerifierAgainstNymEID with invalid input
	verifierEID, err := deserializer.DeserializeVerifierAgainstNymEID(nil, nil)
	require.Error(t, err)
	require.Nil(t, verifierEID)

	// Test DeserializeAuditInfo with invalid input
	auditInfo, err := deserializer.DeserializeAuditInfo(nil)
	require.Error(t, err)
	require.Nil(t, auditInfo)

	// Test Info with invalid input
	infoStr, err := deserializer.Info(nil, nil)
	require.Error(t, err)
	require.Empty(t, infoStr)
}
