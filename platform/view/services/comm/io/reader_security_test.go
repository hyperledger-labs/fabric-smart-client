/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVarintReader_OOMProtection(t *testing.T) {
	// Create a buffer with a huge length prefix (e.g., 1GB)
	hugeLength := uint64(1024 * 1024 * 1024)
	buf := make([]byte, 10)
	n := binary.PutUvarint(buf, hugeLength)

	r := newVarintReader(bytes.NewReader(buf[:n]), 1024)

	data, err := r.ReadData()
	require.Error(t, err)
	require.Nil(t, data)
	require.Contains(t, err.Error(), "exceeds max message size")
}

func TestVarintReader_NormalMessage(t *testing.T) {
	msg := []byte("hello world")
	buf := make([]byte, 20)
	n := binary.PutUvarint(buf, uint64(len(msg)))
	copy(buf[n:], msg)

	r := newVarintReader(bytes.NewReader(buf[:n+len(msg)]), 1024)

	data, err := r.ReadData()
	require.NoError(t, err)
	require.Equal(t, msg, data)
}
