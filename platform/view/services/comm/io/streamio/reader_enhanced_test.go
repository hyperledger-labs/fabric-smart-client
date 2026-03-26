/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package streamio

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReader_ErrorHandling(t *testing.T) {
	t.Run("error from message reader", func(t *testing.T) {
		expectedErr := errors.New("read error")
		mrr := &errorMsgReader{err: expectedErr}
		reader := NewReader(mrr)

		buf := make([]byte, 10)
		n, err := reader.Read(buf)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Equal(t, 0, n)
	})

	t.Run("nil message from reader", func(t *testing.T) {
		mrr := &nilMsgReader{}
		reader := NewReader(mrr)

		buf := make([]byte, 10)
		n, err := reader.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 0, n)
	})

	t.Run("empty message from reader", func(t *testing.T) {
		mrr := newMockMessageReader([][]byte{{}})
		reader := NewReader(mrr)

		buf := make([]byte, 10)
		n, err := reader.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 0, n)
	})
}

func TestReader_BufferManagement(t *testing.T) {
	t.Run("message larger than buffer", func(t *testing.T) {
		largeMsg := []byte("this is a very long message that exceeds buffer size")
		mrr := newMockMessageReader([][]byte{largeMsg})
		reader := NewReader(mrr)

		// Read with small buffer
		buf := make([]byte, 10)

		// First read gets first 10 bytes
		n, err := reader.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 10, n)
		assert.Equal(t, largeMsg[:10], buf)

		// Second read gets next 10 bytes
		n, err = reader.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 10, n)
		assert.Equal(t, largeMsg[10:20], buf)

		// Continue reading until message is exhausted
		totalRead := 20
		for totalRead < len(largeMsg) {
			n, err = reader.Read(buf)
			require.NoError(t, err)
			totalRead += n
		}
		assert.Equal(t, len(largeMsg), totalRead)
	})

	t.Run("buffer larger than message", func(t *testing.T) {
		smallMsg := []byte("small")
		mrr := newMockMessageReader([][]byte{smallMsg})
		reader := NewReader(mrr)

		// Read with large buffer
		buf := make([]byte, 100)
		n, err := reader.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, len(smallMsg), n)
		assert.Equal(t, smallMsg, buf[:n])
	})

	t.Run("exact buffer size match", func(t *testing.T) {
		msg := []byte("exact")
		mrr := newMockMessageReader([][]byte{msg})
		reader := NewReader(mrr)

		buf := make([]byte, len(msg))
		n, err := reader.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, len(msg), n)
		assert.Equal(t, msg, buf)
	})
}

func TestReader_MultipleMessages(t *testing.T) {
	t.Run("sequential reads across messages", func(t *testing.T) {
		messages := [][]byte{
			[]byte("first"),
			[]byte("second"),
			[]byte("third"),
		}
		mrr := newMockMessageReader(messages)
		reader := NewReader(mrr)

		buf := make([]byte, 20)

		// Read all messages
		for _, expected := range messages {
			n, err := reader.Read(buf)
			require.NoError(t, err)
			assert.Equal(t, expected, buf[:n])
		}

		// Next read should return nil/nil (no more messages)
		n, err := reader.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, 0, n)
	})

	t.Run("partial reads across messages", func(t *testing.T) {
		messages := [][]byte{
			[]byte("message1"),
			[]byte("message2"),
		}
		mrr := newMockMessageReader(messages)
		reader := NewReader(mrr)

		buf := make([]byte, 5)

		// Read first part of first message
		n, err := reader.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("messa"), buf)

		// Read rest of first message
		n, err = reader.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 3, n)
		assert.Equal(t, []byte("ge1"), buf[:n])

		// Read first part of second message
		n, err = reader.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("messa"), buf)
	})
}

func TestMD5Hash(t *testing.T) {
	t.Run("hash empty input", func(t *testing.T) {
		hash := MD5Hash([]byte{})
		assert.NotNil(t, hash)
		assert.Len(t, hash, 16) // MD5 produces 16 bytes
	})

	t.Run("hash non-empty input", func(t *testing.T) {
		input := []byte("test data")
		hash := MD5Hash(input)
		assert.NotNil(t, hash)
		assert.Len(t, hash, 16)
	})

	t.Run("same input produces same hash", func(t *testing.T) {
		input := []byte("consistent data")
		hash1 := MD5Hash(input)
		hash2 := MD5Hash(input)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		hash1 := MD5Hash([]byte("data1"))
		hash2 := MD5Hash([]byte("data2"))
		assert.NotEqual(t, hash1, hash2)
	})
}

// Helper types for testing

type errorMsgReader struct {
	err error
}

func (e *errorMsgReader) Read() ([]byte, error) {
	return nil, e.err
}

type nilMsgReader struct{}

func (n *nilMsgReader) Read() ([]byte, error) {
	return nil, nil
}
