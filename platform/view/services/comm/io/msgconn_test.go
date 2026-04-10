/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"context"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMsgConn(t *testing.T) {
	t.Parallel()
	t.Run("successful creation", func(t *testing.T) {
		t.Parallel()
		session := &mockSession{
			ch: make(chan *view.Message, 10),
		}

		conn, err := NewMsgConn(0, session)
		require.NoError(t, err)
		require.NotNil(t, conn)
	})
}

func TestMsgConn_Write(t *testing.T) {
	t.Parallel()
	t.Run("successful write", func(t *testing.T) {
		t.Parallel()
		session := &mockSession{
			ch: make(chan *view.Message, 10),
		}

		conn, err := NewMsgConn(0, session)
		require.NoError(t, err)

		data := []byte("test message")
		n, err := conn.Write(data)
		require.NoError(t, err)
		require.Equal(t, len(data), n)
		require.Len(t, session.sentMessages, 1)
		require.Equal(t, data, session.sentMessages[0])
	})

	t.Run("write empty data", func(t *testing.T) {
		t.Parallel()
		session := &mockSession{
			ch: make(chan *view.Message, 10),
		}

		conn, err := NewMsgConn(0, session)
		require.NoError(t, err)

		n, err := conn.Write([]byte{})
		require.NoError(t, err)
		require.Equal(t, 0, n)
	})

	t.Run("write error", func(t *testing.T) {
		t.Parallel()
		session := &mockSession{
			ch:        make(chan *view.Message, 10),
			sendError: assert.AnError,
		}

		conn, err := NewMsgConn(0, session)
		require.NoError(t, err)

		n, err := conn.Write([]byte("test"))
		require.Error(t, err)
		require.Equal(t, 0, n)
		require.Contains(t, err.Error(), "failed sending message")
	})

	t.Run("multiple writes", func(t *testing.T) {
		t.Parallel()
		session := &mockSession{
			ch: make(chan *view.Message, 10),
		}

		conn, err := NewMsgConn(0, session)
		require.NoError(t, err)

		messages := [][]byte{
			[]byte("first"),
			[]byte("second"),
			[]byte("third"),
		}

		for _, msg := range messages {
			n, err := conn.Write(msg)
			require.NoError(t, err)
			require.Equal(t, len(msg), n)
		}

		require.Len(t, session.sentMessages, len(messages))
		for i, expected := range messages {
			require.Equal(t, expected, session.sentMessages[i])
		}
	})
}

func TestMsgConn_Read(t *testing.T) {
	t.Parallel()
	t.Run("successful read", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 10)
		session := &mockSession{ch: ch}

		conn, err := NewMsgConn(0, session)
		require.NoError(t, err)

		expectedData := []byte("test message")
		ch <- &view.Message{
			Payload: expectedData,
			Status:  view.OK,
		}

		data, err := conn.Read()
		require.NoError(t, err)
		require.Equal(t, expectedData, data)
	})

	t.Run("read multiple messages", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 10)
		session := &mockSession{ch: ch}

		conn, err := NewMsgConn(0, session)
		require.NoError(t, err)

		messages := [][]byte{
			[]byte("first"),
			[]byte("second"),
			[]byte("third"),
		}

		for _, msg := range messages {
			ch <- &view.Message{
				Payload: msg,
				Status:  view.OK,
			}
		}

		msgConn := conn
		for _, expected := range messages {
			data, err := msgConn.Read()
			require.NoError(t, err)
			require.Equal(t, expected, data)
		}
	})

	t.Run("channel closed", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 10)
		session := &mockSession{ch: ch}

		conn, err := NewMsgConn(0, session)
		require.NoError(t, err)

		close(ch)

		data, err := conn.Read()
		require.Error(t, err)
		require.Nil(t, data)
		require.Contains(t, err.Error(), "channel closed")
	})

	t.Run("nil message", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 10)
		session := &mockSession{ch: ch}

		conn, err := NewMsgConn(0, session)
		require.NoError(t, err)

		ch <- nil

		data, err := conn.Read()
		require.Error(t, err)
		require.Nil(t, data)
		require.Contains(t, err.Error(), "received nil message")
	})

	t.Run("error status", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 10)
		session := &mockSession{ch: ch}

		conn, err := NewMsgConn(0, session)
		require.NoError(t, err)

		ch <- &view.Message{
			Payload: []byte("error message"),
			Status:  view.ERROR,
		}

		data, err := conn.Read()
		require.Error(t, err)
		require.Nil(t, data)
		require.Equal(t, "error message", err.Error())
	})

	t.Run("empty payload", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 10)
		session := &mockSession{ch: ch}

		conn, err := NewMsgConn(0, session)
		require.NoError(t, err)

		ch <- &view.Message{
			Payload: []byte{},
			Status:  view.OK,
		}

		data, err := conn.Read()
		require.Error(t, err)
		require.Nil(t, data)
		require.Contains(t, err.Error(), "failed receiving message")
	})
}

func TestMsgConn_Flush(t *testing.T) {
	t.Parallel()
	t.Run("flush always succeeds", func(t *testing.T) {
		t.Parallel()
		session := &mockSession{
			ch: make(chan *view.Message, 10),
		}

		conn, err := NewMsgConn(0, session)
		require.NoError(t, err)

		err = conn.Flush()
		require.NoError(t, err)
	})
}

// Mock session for testing
type mockSession struct {
	ch           chan *view.Message
	sentMessages [][]byte
	sendError    error
	sessionID    string
}

func (m *mockSession) Send(payload []byte) error {
	if m.sendError != nil {
		return m.sendError
	}
	m.sentMessages = append(m.sentMessages, payload)
	return nil
}

func (m *mockSession) Receive() <-chan *view.Message {
	return m.ch
}

func (m *mockSession) Info() view.SessionInfo {
	return view.SessionInfo{}
}

func (m *mockSession) Close() {
	if m.ch != nil {
		close(m.ch)
	}
}

func (m *mockSession) SessionID() string {
	return m.sessionID
}

func (m *mockSession) Context() view.Context {
	return nil
}

func (m *mockSession) SendWithContext(ctx context.Context, payload []byte) error {
	return m.Send(payload)
}

func (m *mockSession) SendError(payload []byte) error {
	if m.sendError != nil {
		return m.sendError
	}
	m.sentMessages = append(m.sentMessages, payload)
	return nil
}

func (m *mockSession) SendErrorWithContext(ctx context.Context, payload []byte) error {
	return m.SendError(payload)
}

func (m *mockSession) ReceiveWithTimeout(timeout time.Duration) (*view.Message, error) {
	select {
	case msg := <-m.ch:
		return msg, nil
	case <-time.After(timeout):
		return nil, context.DeadlineExceeded
	}
}

func (m *mockSession) String() string {
	return "mock-session"
}
