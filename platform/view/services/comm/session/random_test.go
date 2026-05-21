/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/
package session

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetRandomBytes(t *testing.T) {
	t.Parallel()
	b, err := GetRandomBytes(32)
	require.NoError(t, err)
	require.Len(t, b, 32)
}

func TestGetRandomBytesUniqueness(t *testing.T) {
	t.Parallel()
	b1, err := GetRandomBytes(24)
	require.NoError(t, err)
	b2, err := GetRandomBytes(24)
	require.NoError(t, err)
	require.NotEqual(t, b1, b2)
}

func TestGetRandomNonce(t *testing.T) {
	t.Parallel()
	nonce, err := GetRandomNonce()
	require.NoError(t, err)
	require.Len(t, nonce, NonceSize)
}

func TestGetRandomNonceUniqueness(t *testing.T) {
	t.Parallel()
	n1, err := GetRandomNonce()
	require.NoError(t, err)
	n2, err := GetRandomNonce()
	require.NoError(t, err)
	require.NotEqual(t, n1, n2)
}

func TestGetRandomBytesZeroLen(t *testing.T) {
	t.Parallel()
	b, err := GetRandomBytes(0)
	require.NoError(t, err)
	require.Len(t, b, 0)
}
