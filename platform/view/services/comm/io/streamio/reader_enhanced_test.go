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
	t.Parallel()
	t.Run("error from message reader", func(t *testing.T) {
		t.Parallel()
		expectedErr := errors.New("read error")
		mrr := &errorMsgReader{err: expectedErr}
		reader := NewReader(mrr)

		buf := make([]byte, 10)
		n, err := reader.Read(buf)
		require.Error(t, err)
		require.Equal(t, expectedErr, err)
		require.Equal(t, 0, n)
	})

	t.Run("nil message from reader", func(t *testing.T) {
		t.Parallel()
		mrr := &nilMsgReader{}
		reader := NewReader(mrr)

		buf := make([]byte, 10)
		n, err := reader.Read(buf)
		require.NoError(t, err)
		require.Equal(t, 0, n)
	})

	t.Run("empty message from reader", func(t *testing.T) {
		t.Parallel()
		mrr := newMockMessageReader([][]byte{{}})
		reader := NewReader(mrr)

		buf := make([]byte, 10)
		n, err := reader.Read(buf)
		require.NoError(t, err)
		require.Equal(t, 0, n)
	})
}

func TestReader_BufferManagement(t *testing.T) {
	t.Parallel()
	t.Run("message larger than buffer", func(t *testing.T) {
		t.Parallel()
		largeMsg := []byte("this is a very long message that exceeds buffer size")
		mrr := newMockMessageReader([][]byte{largeMsg})
		reader := NewReader(mrr)

		// Read with small buffer
		buf := make([]byte, 10)

		// First read gets first 10 bytes
		n, err := reader.Read(buf)
		require.NoError(t, err)
		require.Equal(t, 10, n)
		require.Equal(t, largeMsg[:10], buf)

		// Second read gets next 10 bytes
		n, err = reader.Read(buf)
		require.NoError(t, err)
		require.Equal(t, 10, n)
		require.Equal(t, largeMsg[10:20], buf)

		// Continue reading until message is exhausted
		totalRead := 20
		for totalRead < len(largeMsg) {
			n, err = reader.Read(buf)
			require.NoError(t, err)
			totalRead += n
		}
		require.Equal(t, len(largeMsg), totalRead)
	})

	t.Run("buffer larger than message", func(t *testing.T) {
		t.Parallel()
		smallMsg := []byte("small")
		mrr := newMockMessageReader([][]byte{smallMsg})
		reader := NewReader(mrr)

		// Read with large buffer
		buf := make([]byte, 100)
		n, err := reader.Read(buf)
		require.NoError(t, err)
		require.Len(t, smallMsg, n)
		require.Equal(t, smallMsg, buf[:n])
	})

	t.Run("exact buffer size match", func(t *testing.T) {
		t.Parallel()
		msg := []byte("exact")
		mrr := newMockMessageReader([][]byte{msg})
		reader := NewReader(mrr)

		buf := make([]byte, len(msg))
		n, err := reader.Read(buf)
		require.NoError(t, err)
		require.Len(t, msg, n)
		require.Equal(t, msg, buf)
	})
}

func TestReader_MultipleMessages(t *testing.T) {
	t.Parallel()
	t.Run("sequential reads across messages", func(t *testing.T) {
		t.Parallel()
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
			require.Equal(t, expected, buf[:n])
		}

		// Next read should return nil/nil (no more messages)
		n, err := reader.Read(buf)
		require.NoError(t, err)
		require.Equal(t, 0, n)
	})

	t.Run("partial reads across messages", func(t *testing.T) {
		t.Parallel()
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
