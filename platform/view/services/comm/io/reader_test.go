/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestVarintReader_ReadData(t *testing.T) {
	t.Run("successful read", func(t *testing.T) {
		data := []byte("test message")
		buf := &bytes.Buffer{}

		// Write length prefix
		lenBuf := make([]byte, binary.MaxVarintLen64)
		n := binary.PutUvarint(lenBuf, uint64(len(data)))
		buf.Write(lenBuf[:n])
		buf.Write(data)

		r := newVarintReader(buf, 1024)
		readData, err := r.ReadData()
		require.NoError(t, err)
		require.Equal(t, data, readData)
	})

	t.Run("read empty data", func(t *testing.T) {
		buf := &bytes.Buffer{}
		lenBuf := make([]byte, binary.MaxVarintLen64)
		n := binary.PutUvarint(lenBuf, 0)
		buf.Write(lenBuf[:n])

		r := newVarintReader(buf, 1024)
		readData, err := r.ReadData()
		require.NoError(t, err)
		require.Equal(t, []byte{}, readData)
	})

	t.Run("read multiple messages", func(t *testing.T) {
		messages := [][]byte{
			[]byte("first"),
			[]byte("second"),
			[]byte("third"),
		}

		buf := &bytes.Buffer{}
		for _, msg := range messages {
			lenBuf := make([]byte, binary.MaxVarintLen64)
			n := binary.PutUvarint(lenBuf, uint64(len(msg)))
			buf.Write(lenBuf[:n])
			buf.Write(msg)
		}

		r := newVarintReader(buf, 1024)
		for i, expected := range messages {
			readData, err := r.ReadData()
			require.NoError(t, err, "failed reading message %d", i)
			require.Equal(t, expected, readData)
		}
	})

	t.Run("error on EOF reading length", func(t *testing.T) {
		buf := &bytes.Buffer{}
		r := newVarintReader(buf, 1024)

		_, err := r.ReadData()
		require.Error(t, err)
		require.Equal(t, io.EOF, err)
	})

	t.Run("error on incomplete message", func(t *testing.T) {
		buf := &bytes.Buffer{}
		lenBuf := make([]byte, binary.MaxVarintLen64)
		n := binary.PutUvarint(lenBuf, 10)
		buf.Write(lenBuf[:n])
		buf.Write([]byte("short")) // Only 5 bytes instead of 10

		r := newVarintReader(buf, 1024)
		_, err := r.ReadData()
		require.Error(t, err)
		require.Contains(t, err.Error(), "error reading message")
	})
}

func TestVarintReader_Close(t *testing.T) {
	t.Run("close with closer", func(t *testing.T) {
		closer := &mockReadCloser{Buffer: bytes.NewBuffer(nil)}
		r := newVarintReader(closer, 1024)

		err := r.Close()
		require.NoError(t, err)
		require.True(t, closer.closed)
	})

	t.Run("close without closer", func(t *testing.T) {
		buf := &bytes.Buffer{}
		r := newVarintReader(buf, 1024)

		err := r.Close()
		require.NoError(t, err)
	})

	t.Run("close error", func(t *testing.T) {
		closer := &mockReadCloser{
			Buffer:   bytes.NewBuffer(nil),
			closeErr: assert.AnError,
		}
		r := newVarintReader(closer, 1024)

		err := r.Close()
		require.Error(t, err)
		require.Equal(t, assert.AnError, err)
	})
}

func TestProtoReader_ReadMsg(t *testing.T) {
	t.Run("successful read", func(t *testing.T) {
		msg := &anypb.Any{
			TypeUrl: "test.type",
			Value:   []byte("test value"),
		}

		buf := &bytes.Buffer{}
		w := NewVarintProtoWriter(buf)
		err := w.WriteMsg(msg)
		require.NoError(t, err)

		r := NewVarintProtoReader(buf, 1024)
		readMsg := &anypb.Any{}
		err = r.ReadMsg(readMsg)
		require.NoError(t, err)
		require.Equal(t, msg.TypeUrl, readMsg.TypeUrl)
		require.Equal(t, msg.Value, readMsg.Value)
	})

	t.Run("read multiple messages", func(t *testing.T) {
		messages := []*anypb.Any{
			{TypeUrl: "type1", Value: []byte("value1")},
			{TypeUrl: "type2", Value: []byte("value2")},
			{TypeUrl: "type3", Value: []byte("value3")},
		}

		buf := &bytes.Buffer{}
		w := NewVarintProtoWriter(buf)
		for _, msg := range messages {
			err := w.WriteMsg(msg)
			require.NoError(t, err)
		}

		r := NewVarintProtoReader(buf, 1024)
		for i, expected := range messages {
			readMsg := &anypb.Any{}
			err := r.ReadMsg(readMsg)
			require.NoError(t, err, "failed reading message %d", i)
			require.Equal(t, expected.TypeUrl, readMsg.TypeUrl)
			require.Equal(t, expected.Value, readMsg.Value)
		}
	})

	t.Run("error on EOF", func(t *testing.T) {
		buf := &bytes.Buffer{}
		r := NewVarintProtoReader(buf, 1024)

		msg := &anypb.Any{}
		err := r.ReadMsg(msg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed reading data")
	})

	t.Run("error on invalid protobuf", func(t *testing.T) {
		buf := &bytes.Buffer{}
		// Write invalid protobuf data
		lenBuf := make([]byte, binary.MaxVarintLen64)
		invalidData := []byte("not a valid protobuf")
		n := binary.PutUvarint(lenBuf, uint64(len(invalidData)))
		buf.Write(lenBuf[:n])
		buf.Write(invalidData)

		r := NewVarintProtoReader(buf, 1024)
		msg := &anypb.Any{}
		err := r.ReadMsg(msg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed unmarshalling message")
	})
}

func TestProtoReader_Close(t *testing.T) {
	t.Run("close successfully", func(t *testing.T) {
		closer := &mockReadCloser{Buffer: bytes.NewBuffer(nil)}
		r := NewVarintProtoReader(closer, 1024)

		err := r.Close()
		require.NoError(t, err)
		require.True(t, closer.closed)
	})

	t.Run("close without closer", func(t *testing.T) {
		buf := &bytes.Buffer{}
		r := NewVarintProtoReader(buf, 1024)

		err := r.Close()
		require.NoError(t, err)
	})
}

func TestNewVarintProtoReader(t *testing.T) {
	buf := &bytes.Buffer{}
	r := NewVarintProtoReader(buf, 1024)
	require.NotNil(t, r)
}

func TestRoundTrip(t *testing.T) {
	t.Run("write and read back", func(t *testing.T) {
		buf := &bytes.Buffer{}

		// Write messages
		w := NewVarintProtoWriter(buf)
		messages := []*anypb.Any{
			{TypeUrl: "type1", Value: []byte("value1")},
			{TypeUrl: "type2", Value: []byte("value2")},
		}

		for _, msg := range messages {
			err := w.WriteMsg(msg)
			require.NoError(t, err)
		}

		// Read messages back
		r := NewVarintProtoReader(buf, 1024)
		for i, expected := range messages {
			readMsg := &anypb.Any{}
			err := r.ReadMsg(readMsg)
			require.NoError(t, err, "failed reading message %d", i)
			require.Equal(t, expected.TypeUrl, readMsg.TypeUrl)
			require.Equal(t, expected.Value, readMsg.Value)
		}
	})
}

// Helper types for testing

type mockReadCloser struct {
	*bytes.Buffer
	closed   bool
	closeErr error
}

func (m *mockReadCloser) Close() error {
	m.closed = true
	return m.closeErr
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	if m.Buffer == nil {
		return 0, io.EOF
	}
	return m.Buffer.Read(p)
}
