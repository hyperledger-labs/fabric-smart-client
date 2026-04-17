/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestSessionDeadlockDueToSlowReceiver(t *testing.T) { //nolint:paralleltest
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Create a session with unbuffered incoming channel to block the goroutine on send.
	// middleCh is buffered with size 1.
	s := &NetworkStreamSession{
		node:            &mockSenderDeadlock{},
		endpointID:      []byte("endpointID"),
		endpointAddress: "endpointAddress",
		contextID:       "contextID",
		sessionID:       "sessionID",
		caller:          []byte("caller"),
		callerViewID:    "callerViewID",
		incoming:        make(chan *view.Message), // unbuffered
		streams:         make(map[*streamHandler]struct{}),
		middleCh:        make(chan *view.Message, 1),
		closing:         make(chan struct{}),
		closed:          make(chan struct{}),
	}
	// Hide the implementation behind the session interface.
	var sess view.Session = s

	// Start the session by enqueuing a message.
	msg1 := &view.Message{Payload: []byte("msg1")}
	ok := s.enqueue(msg1)
	require.True(t, ok, "First enqueue should succeed")

	// Now, the goroutine has received msg1 from middleCh and is blocked on sending to incoming
	// because incoming is unbuffered and we are not reading from the session's Receive() channel.

	// The middleCh is now empty because the goroutine consumed msg1.

	// Now, we fill the middleCh with one message (the buffer size).
	// This will block because the goroutine is not reading from middleCh (it's blocked on incoming).
	done := make(chan struct{})
	go func() {
		s.middleCh <- &view.Message{Payload: []byte("msg2")}
		close(done)
	}()
	// Wait a bit for the goroutine to start the send.
	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
	}

	// Now, try to enqueue one more message. This should block on sending to middleCh
	// because the goroutine is not reading from middleCh (it's blocked on sending to incoming)
	// and middleCh is full (has the msg2 from the goroutine above).
	msg := &view.Message{Payload: []byte("msg")}
	ok = s.enqueueWithTimeout(msg, 100*time.Millisecond)
	require.False(t, ok, "Enqueue should timeout due to full middleCh and blocked goroutine")

	// The session should be closed due to the timeout in enqueueWithTimeout.
	require.True(t, s.isClosed(), "Session should be closed after enqueue timeout")

	// Verify that we cannot enqueue any more messages.
	require.False(t, s.enqueue(&view.Message{Payload: []byte("msg3")}), "Enqueue on closed session should return false")
	_ = sess
}

func TestSessionDeadlockDueToMiddlewareChannelFull(t *testing.T) { //nolint:paralleltest
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Create a session with unbuffered incoming channel to block the goroutine on send.
	// middleCh is buffered with size 1.
	s := &NetworkStreamSession{
		node:            &mockSenderDeadlock{},
		endpointID:      []byte("endpointID"),
		endpointAddress: "endpointAddress",
		contextID:       "contextID",
		sessionID:       "sessionID",
		caller:          []byte("caller"),
		callerViewID:    "callerViewID",
		incoming:        make(chan *view.Message), // unbuffered
		streams:         make(map[*streamHandler]struct{}),
		middleCh:        make(chan *view.Message, 1),
		closing:         make(chan struct{}),
		closed:          make(chan struct{}),
	}
	var sess view.Session = s

	// Start the session by enqueuing a message.
	msg1 := &view.Message{Payload: []byte("msg1")}
	ok := s.enqueue(msg1)
	require.True(t, ok, "First enqueue should succeed")

	// Now, the goroutine has received msg1 from middleCh and is blocked on sending to incoming
	// because incoming is unbuffered and we are not reading from the session's Receive() channel.

	// The middleCh is now empty because the goroutine consumed msg1.

	// Now, we fill the middleCh with one message (the buffer size).
	// This will block because the goroutine is not reading from middleCh (it's blocked on incoming).
	done := make(chan struct{})
	go func() {
		s.middleCh <- &view.Message{Payload: []byte("msg2")}
		close(done)
	}()
	// Wait a bit for the goroutine to start the send.
	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
	}

	// Now, the middleCh is full (has msg2) and the goroutine is blocked on sending to incoming.

	// Now, we try to enqueue one more message. This should block on sending to middleCh
	// because the goroutine is not reading from middleCh (it's blocked on sending to incoming)
	// and middleCh is full.
	msg := &view.Message{Payload: []byte("msg")}
	ok = s.enqueueWithTimeout(msg, 100*time.Millisecond)
	require.False(t, ok, "Enqueue should timeout due to full middleCh and blocked goroutine")

	// The session should be closed due to the timeout in enqueueWithTimeout.
	require.True(t, s.isClosed(), "Session should be closed after enqueue timeout")

	// Verify that we cannot enqueue any more messages.
	require.False(t, s.enqueue(&view.Message{Payload: []byte("msg3")}), "Enqueue on closed session should return false")
	_ = sess
}

func TestSessionRecoverAfterClose(t *testing.T) { //nolint:paralleltest
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	s := &NetworkStreamSession{
		node:            &mockSenderDeadlock{},
		endpointID:      []byte("endpointID"),
		endpointAddress: "endpointAddress",
		contextID:       "contextID",
		sessionID:       "sessionID",
		caller:          []byte("caller"),
		callerViewID:    "callerViewID",
		incoming:        make(chan *view.Message, 10),
		streams:         make(map[*streamHandler]struct{}),
		middleCh:        make(chan *view.Message, 10),
		closing:         make(chan struct{}),
		closed:          make(chan struct{}),
	}
	var sess view.Session = s

	// Enqueue a message and receive it.
	msg := &view.Message{Payload: []byte("msg")}
	require.True(t, s.enqueue(msg))
	received := <-sess.Receive()
	require.Equal(t, msg, received)

	// Close the session.
	sess.Close()
	require.True(t, sess.Info().Closed)

	// After closing, we should not be able to send or receive.
	require.ErrorIs(t, sess.Send([]byte("data")), ErrSessionClosed)
	require.ErrorIs(t, sess.SendError([]byte("error")), ErrSessionClosed)
	require.False(t, s.enqueue(&view.Message{Payload: []byte("msg2")}))

	// Now, create a new session with the same parameters (simulating a reconnect).
	s2 := &NetworkStreamSession{
		node:            &mockSenderDeadlock{},
		endpointID:      []byte("endpointID"),
		endpointAddress: "endpointAddress",
		contextID:       "contextID",
		sessionID:       "sessionID2", // different session ID
		caller:          []byte("caller"),
		callerViewID:    "callerViewID",
		incoming:        make(chan *view.Message, 10),
		streams:         make(map[*streamHandler]struct{}),
		middleCh:        make(chan *view.Message, 10),
		closing:         make(chan struct{}),
		closed:          make(chan struct{}),
	}
	var sess2 view.Session = s2

	// The new session should work.
	require.True(t, s2.enqueue(&view.Message{Payload: []byte("msg3")}))
	received2 := <-sess2.Receive()
	require.Equal(t, &view.Message{Payload: []byte("msg3")}, received2)

	// Close the new session.
	sess2.Close()
	_ = sess
	_ = sess2
}

// mockSenderDeadlock implements the sender interface for testing.
type mockSenderDeadlock struct{}

func (m *mockSenderDeadlock) sendTo(ctx context.Context, info host2.StreamInfo, msg proto.Message, session *NetworkStreamSession) error {
	// For the purpose of this test, we just need to avoid errors.
	return nil
}
