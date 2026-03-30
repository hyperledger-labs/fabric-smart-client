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

func TestVarintWriter_WriteData(t *testing.T) {
	t.Run("successful write", func(t *testing.T) {
		buf := &bytes.Buffer{}
		w := newVarintWriter(buf)

		data := []byte("test message")
		err := w.WriteData(data)
		require.NoError(t, err)

		// Verify the written data
		written := buf.Bytes()

		// Read the length prefix
		length, n := binary.Uvarint(written)
		assert.Equal(t, uint64(len(data)), length)

		// Verify the actual data
		actualData := written[n:]
		assert.Equal(t, data, actualData)
	})

	t.Run("write empty data", func(t *testing.T) {
		buf := &bytes.Buffer{}
		w := newVarintWriter(buf)

		err := w.WriteData([]byte{})
		require.NoError(t, err)

		written := buf.Bytes()
		length, _ := binary.Uvarint(written)
		assert.Equal(t, uint64(0), length)
	})

	t.Run("write large data", func(t *testing.T) {
		buf := &bytes.Buffer{}
		w := newVarintWriter(buf)

		data := make([]byte, 10000)
		for i := range data {
			data[i] = byte(i % 256)
		}

		err := w.WriteData(data)
		require.NoError(t, err)

		written := buf.Bytes()
		length, n := binary.Uvarint(written)
		assert.Equal(t, uint64(len(data)), length)
		assert.Equal(t, data, written[n:])
	})

	t.Run("write error on length", func(t *testing.T) {
		w := newVarintWriter(&errorWriter{failOn: 0})
		err := w.WriteData([]byte("test"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not write message length")
	})

	t.Run("write error on data", func(t *testing.T) {
		w := newVarintWriter(&errorWriter{failOn: 1})
		err := w.WriteData([]byte("test"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not write data")
	})
}

func TestVarintWriter_Close(t *testing.T) {
	t.Run("close with closer", func(t *testing.T) {
		closer := &mockCloser{Buffer: &bytes.Buffer{}}
		w := newVarintWriter(closer)

		err := w.Close()
		assert.NoError(t, err)
		assert.True(t, closer.closed)
	})

	t.Run("close without closer", func(t *testing.T) {
		buf := &bytes.Buffer{}
		w := newVarintWriter(buf)

		err := w.Close()
		assert.NoError(t, err)
	})

	t.Run("close error", func(t *testing.T) {
		closer := &mockCloser{Buffer: &bytes.Buffer{}, closeErr: assert.AnError}
		w := newVarintWriter(closer)

		err := w.Close()
		assert.Error(t, err)
		assert.Equal(t, assert.AnError, err)
	})
}

func TestProtoWriter_WriteMsg(t *testing.T) {
	t.Run("successful write", func(t *testing.T) {
		buf := &bytes.Buffer{}
		w := NewVarintProtoWriter(buf)

		msg := &anypb.Any{
			TypeUrl: "test.type",
			Value:   []byte("test value"),
		}

		err := w.WriteMsg(msg)
		require.NoError(t, err)

		// Verify we can read it back
		r := NewVarintProtoReader(buf, 1024)
		readMsg := &anypb.Any{}
		err = r.ReadMsg(readMsg)
		require.NoError(t, err)
		assert.Equal(t, msg.TypeUrl, readMsg.TypeUrl)
		assert.Equal(t, msg.Value, readMsg.Value)
	})

	t.Run("write multiple messages", func(t *testing.T) {
		buf := &bytes.Buffer{}
		w := NewVarintProtoWriter(buf)

		messages := []*anypb.Any{
			{TypeUrl: "type1", Value: []byte("value1")},
			{TypeUrl: "type2", Value: []byte("value2")},
			{TypeUrl: "type3", Value: []byte("value3")},
		}

		for _, msg := range messages {
			err := w.WriteMsg(msg)
			require.NoError(t, err)
		}

		// Read them back
		r := NewVarintProtoReader(buf, 1024)
		for i, expected := range messages {
			readMsg := &anypb.Any{}
			err := r.ReadMsg(readMsg)
			require.NoError(t, err, "failed reading message %d", i)
			assert.Equal(t, expected.TypeUrl, readMsg.TypeUrl)
			assert.Equal(t, expected.Value, readMsg.Value)
		}
	})
}

func TestProtoWriter_Close(t *testing.T) {
	t.Run("close successfully", func(t *testing.T) {
		closer := &mockCloser{Buffer: &bytes.Buffer{}}
		w := NewVarintProtoWriter(closer)

		err := w.Close()
		assert.NoError(t, err)
		assert.True(t, closer.closed)
	})

	t.Run("close without closer", func(t *testing.T) {
		buf := &bytes.Buffer{}
		w := NewVarintProtoWriter(buf)

		err := w.Close()
		assert.NoError(t, err)
	})
}

func TestNewVarintProtoWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	w := NewVarintProtoWriter(buf)
	assert.NotNil(t, w)

	// Verify it's functional
	msg := &anypb.Any{TypeUrl: "test", Value: []byte("data")}
	err := w.WriteMsg(msg)
	assert.NoError(t, err)
}

// Helper types for testing

type errorWriter struct {
	failOn int
	count  int
}

func (w *errorWriter) Write(p []byte) (n int, err error) {
	if w.count == w.failOn {
		return 0, assert.AnError
	}
	w.count++
	return len(p), nil
}

type mockCloser struct {
	*bytes.Buffer
	closed   bool
	closeErr error
}

func (m *mockCloser) Close() error {
	m.closed = true
	return m.closeErr
}

func (m *mockCloser) Write(p []byte) (n int, err error) {
	if m.Buffer == nil {
		return 0, io.ErrClosedPipe
	}
	return m.Buffer.Write(p)
}
