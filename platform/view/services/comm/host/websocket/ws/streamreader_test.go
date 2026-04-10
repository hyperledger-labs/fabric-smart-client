/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadExpectedLengthHappyPath(t *testing.T) {
	t.Parallel()
	reader := newDelimitedReader()
	buf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(buf, 128)

	read, err := reader.Read(buf[:n])
	require.NoError(t, err)
	require.Equal(t, n, read)
	require.Equal(t, reader.expectedLength, 128+n)
}

func TestReadExpectedLengthOversizedPayload(t *testing.T) {
	t.Parallel()
	reader := newDelimitedReader()
	buf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(buf, maxDelimitedPayloadSize+1)

	_, err := reader.Read(buf[:n])
	require.Error(t, err)
	require.Contains(t, err.Error(), "message payload too large")
}

func TestReadExpectedLengthPartialVarint(t *testing.T) {
	t.Parallel()
	reader := newDelimitedReader()

	// 128 in varint is [0x80, 0x01]
	// Send first byte
	n, err := reader.Read([]byte{0x80})
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, 0, reader.expectedLength)

	// Send second byte
	n, err = reader.Read([]byte{0x01})
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, 130, reader.expectedLength) // 128 + 2 bytes header
}
