/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestPingPongSessionLevel(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Test ping-pong behavior at the session level with multiple sessions
	const numSessions = 3
	const msgsPerSession = 5

	var wg sync.WaitGroup
	wg.Add(2) // Two goroutines: one for sending, one for receiving

	// Track message counts for verification
	var sentCount int64
	var receivedCount int64

	// Create a session with buffered channels to allow multiple messages in flight
	session := &NetworkStreamSession{
		node:            &pingPongMockNode{latency: 10 * time.Millisecond}, // Mock node that simulates round-trip
		endpointID:      []byte("test-endpoint"),
		endpointAddress: "test-address",
		contextID:       "test-context",
		sessionID:       "test-session",
		caller:          []byte("test-caller"),
		callerViewID:    "test-view",
		incoming:        make(chan *view.Message, 100), // Buffered channel
		streams:         make(map[*streamHandler]struct{}),
		middleCh:        make(chan *view.Message, 100), // Buffered channel
		closing:         make(chan struct{}),
		closed:          make(chan struct{}),
	}

	// Hide implementation behind session interface
	var sess view.Session = session

	// Sender goroutine: sends ping messages
	go func() {
		defer wg.Done()
		for sessionID := 0; sessionID < numSessions; sessionID++ {
			// sessionIDStr := fmt.Sprintf("session-%d", sessionID) // Not used, but kept for clarity

			// Update session ID for this round (in real usage, we'd create new sessions)
			// For this test, we'll just use the same session and vary the message content
			for i := 0; i < msgsPerSession; i++ {
				msg := fmt.Sprintf("ping-%d-%d", sessionID, i)
				err := sess.Send([]byte(msg))
				assert.NoError(t, err, "Failed to send message")
				atomic.AddInt64(&sentCount, 1)

				// Small delay to simulate processing time
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()

	// Receiver goroutine: receives messages and sends pongs back
	go func() {
		defer wg.Done()
		received := make(chan *view.Message, 100)

		// Start a goroutine to collect messages from the session
		go func() {
			for msg := range sess.Receive() {
				received <- msg
			}
		}()

		// Process received messages and send pongs
		for i := 0; i < numSessions*msgsPerSession; i++ {
			select {
			case msg := <-received:
				// Verify we got a ping message
				assert.Contains(t, string(msg.Payload), "ping-", "Expected ping message")

				// Send a pong back
				pongMsg := fmt.Sprintf("pong-%s", string(msg.Payload))
				err := sess.Send([]byte(pongMsg))
				assert.NoError(t, err, "Failed to send pong message")
				atomic.AddInt64(&receivedCount, 1)

				// Small delay to simulate processing time
				time.Sleep(5 * time.Millisecond)
			case <-time.After(2 * time.Second):
				t.Error("Timeout waiting for message")
				return
			}
		}
	}()

	// Wait for both sender and receiver to complete
	wg.Wait()

	// Close the session to clean up
	sess.Close()

	// Give time for cleanup
	time.Sleep(100 * time.Millisecond)

	// Verify that we sent and received the expected number of messages
	expectedTotal := int64(numSessions * msgsPerSession)
	require.Equal(t, expectedTotal, atomic.LoadInt64(&sentCount),
		"Should have sent expected number of ping messages")
	require.Equal(t, expectedTotal, atomic.LoadInt64(&receivedCount),
		"Should have received and replied with expected number of pong messages")
}

// Mock node that simulates round-trip message delivery
type pingPongMockNode struct {
	// We'll use a simple approach: when a message is sent to us,
	// we'll send it back after a short delay to simulate network round-trip
	latency time.Duration
}

func (m *pingPongMockNode) sendTo(ctx context.Context, info host2.StreamInfo, msg proto.Message, session *NetworkStreamSession) error {
	// In a real implementation, this would send the message over the network
	// For our test, we'll simulate network latency by sending the message back
	// after a short delay

	// Create a response message
	go func() {
		// Simulate network latency using the configured latency
		time.Sleep(m.latency)

		// Create a response message that looks like a pong
		response := &view.Message{
			ContextID: msg.(*ViewPacket).ContextID,
			SessionID: msg.(*ViewPacket).SessionID,
			Caller:    "ponger", // Indicate this is a pong
			Payload:   append([]byte("pong-"), msg.(*ViewPacket).Payload...),
			Status:    msg.(*ViewPacket).Status,
		}

		// Enqueue the response back to the session
		if !session.isClosed() {
			session.enqueue(response)
		}
	}()

	return nil
}
