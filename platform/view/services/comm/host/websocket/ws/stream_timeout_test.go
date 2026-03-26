/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws_test

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/ws"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// TestWebSocketStreamDeliveryTimeout tests the stream delivery timeout in the deliver function
func TestWebSocketStreamDeliveryTimeout(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Create a mock connection that simulates a hung read operation
	mockConn := &mockWebSocketConn{
		readCh:  make(chan []byte, 1),
		writeCh: make(chan []byte, 1),
		mu:      sync.Mutex{},
		// Set initial deadlines far in the future so they don't interfere with context-based tests
		readDeadline:  time.Now().Add(24 * time.Hour),
		writeDeadline: time.Now().Add(24 * time.Hour),
	}

	// Don't send any data on readCh, so ReadMessage will block

	streamInfo := host2.StreamInfo{
		RemotePeerID:      "test-peer",
		RemotePeerAddress: "127.0.0.1:8080",
		ContextID:         "test-context",
		SessionID:         "test-session",
	}

	// Try to read from the stream - should fail due to context timeout (since we set a short context)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	stream := ws.NewWSStream(mockConn, ctx, streamInfo)

	_, err := stream.Read([]byte("test"))
	require.Error(t, err)
	// Should be either context deadline exceeded or EOF (due to a race in the test)
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, io.EOF) {
		t.Fatalf("expected context deadline exceeded or EOF, got: %v", err)
	}
}

// TestWebSocketContextCancellationWithTimeouts tests that context cancellation works alongside timeouts
func TestWebSocketContextCancellationWithTimeouts(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Create a mock connection
	mockConn := &mockWebSocketConn{
		readCh:  make(chan []byte, 1),
		writeCh: make(chan []byte, 1),
		mu:      sync.Mutex{},
		// Set initial deadlines far in the future so they don't interfere with context-based tests
		readDeadline:  time.Now().Add(24 * time.Hour),
		writeDeadline: time.Now().Add(24 * time.Hour),
	}

	// Don't send any data on readCh, so ReadMessage will block

	streamInfo := host2.StreamInfo{
		RemotePeerID:      "test-peer",
		RemotePeerAddress: "127.0.0.1:8080",
		ContextID:         "test-context",
		SessionID:         "test-session",
	}

	// Create a context that will be cancelled quickly
	ctx, cancel := context.WithCancel(t.Context())
	// Cancel almost immediately
	time.AfterFunc(5*time.Millisecond, cancel)

	stream := ws.NewWSStream(mockConn, ctx, streamInfo)

	// Try to read - should fail due to context cancellation
	_, err := stream.Read([]byte("test"))
	require.Error(t, err)
	// Should be either context canceled or EOF (due to a race in the test)
	if !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		t.Fatalf("expected context canceled or EOF, got: %v", err)
	}
}

// TestWebSocketPeerCloseRace fixes a race condition where if a peer sends a message and then
// immediately closes the connection, the receiver might get a context cancellation error
// instead of receiving the message.
func TestWebSocketPeerCloseRace(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Create a mock connection
	mockConn := &mockWebSocketConn{
		readCh:  make(chan []byte, 1),
		writeCh: make(chan []byte, 1),
		mu:      sync.Mutex{},
		// Set initial deadlines far in the future so they don't interfere with context-based tests
		readDeadline:  time.Now().Add(24 * time.Hour),
		writeDeadline: time.Now().Add(24 * time.Hour),
	}

	streamInfo := host2.StreamInfo{
		RemotePeerID:      "test-peer",
		RemotePeerAddress: "127.0.0.1:8080",
		ContextID:         "test-context",
		SessionID:         "test-session",
	}

	// Use a context with timeout to simulate connection issues
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	stream := ws.NewWSStream(mockConn, ctx, streamInfo)

	// Send a message from the peer to us (simulate peer sending data to us)
	testMsg := []byte("hello peer")
	mockConn.readCh <- testMsg

	// Give the stream a moment to process the message before simulating connection closure
	// In a real scenario, there would be some delay between receiving the message and connection closure
	time.Sleep(10 * time.Millisecond)

	// Simulate connection closure by making ReadMessage return an error
	// Before the fix, this would fail with context.Canceled due to the race condition
	// Close the mock connection to make ReadMessage return net.ErrClosed
	_ = mockConn.Close()

	// Try to read the message - should succeed and get the message
	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	require.NoError(t, err, "Should successfully read message before context cancellation")
	require.Equal(t, len(testMsg), n, "Should read the correct number of bytes")
	require.Equal(t, testMsg, buf[:n], "Should read the correct message data")

	// Any subsequent read should fail with context cancellation, context deadline exceeded, or EOF
	_, err = stream.Read(buf)
	require.Error(t, err)
	// Should be either context canceled, context deadline exceeded, or EOF
	require.True(t, errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF),
		"Subsequent reads should fail with context canceled, deadline exceeded, or EOF, got: %v", err)
}

// mockWebSocketConn is a minimal mock of websocket.Conn for testing
type mockWebSocketConn struct {
	readCh        chan []byte
	writeCh       chan []byte
	mu            sync.Mutex
	readDeadline  time.Time
	writeDeadline time.Time
	closed        bool
}

// SetReadDeadline implements the websocket.Conn interface
func (m *mockWebSocketConn) SetReadDeadline(t time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readDeadline = t
	return nil
}

// SetWriteDeadline implements the websocket.Conn interface
func (m *mockWebSocketConn) SetWriteDeadline(t time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeDeadline = t
	return nil
}

// ReadMessage implements the websocket.Conn interface
func (m *mockWebSocketConn) ReadMessage() (int, []byte, error) {
	// Block waiting for data to be available on readCh
	// This simulates waiting for data from the peer
	m.mu.Lock()
	deadline := m.readDeadline
	m.mu.Unlock()

	select {
	case data, ok := <-m.readCh:
		if !ok {
			// Channel closed
			return websocket.TextMessage, nil, net.ErrClosed
		}
		return websocket.TextMessage, data, nil
	case <-time.After(time.Until(deadline)):
		// Read timeout occurred
		return websocket.TextMessage, nil, net.ErrClosed
	}
}

// WriteMessage implements the websocket.Conn interface
func (m *mockWebSocketConn) WriteMessage(messageType int, data []byte) error {
	// Simulate writing by sending to writeCh
	select {
	case m.writeCh <- data:
		return nil
	case <-time.After(time.Until(m.writeDeadline)):
		// Write timeout occurred
		return net.ErrClosed
	case <-time.After(1 * time.Second):
		// Timeout if blocked too long (fallback)
		return net.ErrClosed
	}
}

// Close implements the websocket.Conn interface
func (m *mockWebSocketConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	close(m.readCh)
	close(m.writeCh)
	return nil
}
