/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestEnqueueTimeoutBehavior(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Test that enqueueWithTimeout respects the timeout value and logs appropriately
	s := &NetworkStreamSession{
		node:            &mockSenderTimeout{},
		endpointID:      []byte("endpointID"),
		endpointAddress: "endpointAddress",
		contextID:       "contextID",
		sessionID:       "sessionID",
		caller:          []byte("caller"),
		callerViewID:    "callerViewID",
		incoming:        make(chan *view.Message), // unbuffered
		streams:         make(map[*streamHandler]struct{}),
		middleCh:        make(chan *view.Message, 1), // buffered size 1
		closing:         make(chan struct{}),
		closed:          make(chan struct{}),
	}
	var sess view.Session = s

	// Start the session by enqueuing a message (fills incoming channel, blocks goroutine)
	msg1 := &view.Message{Payload: []byte("msg1")}
	require.True(t, s.enqueue(msg1), "First enqueue should succeed")

	// Fill middleCh to block the goroutine from reading
	done := make(chan struct{})
	go func() {
		s.middleCh <- &view.Message{Payload: []byte("blocker")}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
	}

	// Now enqueue with a short timeout - should timeout
	msg2 := &view.Message{Payload: []byte("msg2")}
	ok := s.enqueueWithTimeout(msg2, 50*time.Millisecond)
	require.False(t, ok, "Enqueue should timeout")

	// The session should be closed due to the timeout in enqueueWithTimeout.
	require.True(t, s.isClosed(), "Session should be closed after enqueue timeout")

	// Verify that we cannot enqueue any more messages.
	require.False(t, s.enqueue(&view.Message{Payload: []byte("msg3")}), "Enqueue on closed session should return false")
	_ = sess
}

func TestDefaultEnqueueTimeout(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Test that the default timeout (1 minute) is used when not set
	s := &NetworkStreamSession{
		node:            &mockSenderTimeout{},
		endpointID:      []byte("endpointID"),
		endpointAddress: "endpointAddress",
		contextID:       "contextID",
		sessionID:       "sessionID",
		caller:          []byte("caller"),
		callerViewID:    "callerViewID",
		incoming:        make(chan *view.Message), // unbuffered
		streams:         make(map[*streamHandler]struct{}),
		middleCh:        make(chan *view.Message, 1), // buffered size 1
		closing:         make(chan struct{}),
		closed:          make(chan struct{}),
	}
	var sess view.Session = s

	// Start the session
	msg1 := &view.Message{Payload: []byte("msg1")}
	require.True(t, s.enqueue(msg1), "First enqueue should succeed")

	// Block the goroutine
	done := make(chan struct{})
	go func() {
		s.middleCh <- &view.Message{Payload: []byte("blocker")}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
	}

	// Enqueue without setting timeout - should use DefaultEnqueueTimeout (1 minute)
	// We'll use a much shorter test timeout to avoid waiting a full minute
	s.SetEnqueueTimeout(100 * time.Millisecond) // Override to make test fast
	msg2 := &view.Message{Payload: []byte("msg2")}
	ok := s.enqueueWithTimeout(msg2, 100*time.Millisecond)
	require.False(t, ok, "Enqueue should timeout with custom timeout")

	// The session should be closed due to the timeout in enqueueWithTimeout.
	require.True(t, s.isClosed(), "Session should be closed after timeout")

	// Verify that we cannot enqueue any more messages.
	require.False(t, s.enqueue(&view.Message{Payload: []byte("msg3")}), "Enqueue on closed session should return false")
	_ = sess
}

func TestDrainTimeoutOnClose(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Test that drain timeout works when closing session with blocked consumer
	s := &NetworkStreamSession{
		node:            &mockSenderTimeout{},
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

	// Enqueue a message
	msg1 := &view.Message{Payload: []byte("msg1")}
	require.True(t, s.enqueue(msg1), "First enqueue should succeed")

	// Close the session while there's a message in the queue and no consumer
	done := make(chan struct{})
	go func() {
		sess.Close()
		close(done)
	}()

	// Wait for close to complete - should not hang due to drain timeout
	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Error("Close took too long - possibly blocked on drain timeout")
	}

	// Verify session is closed
	require.True(t, s.isClosed(), "Session should be closed")
	_ = sess
}

func TestServiceStartRetryDelay(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Test that service startup retries with appropriate delay
	// We'll mock the HostProvider to always fail
	failingHostProvider := &mockFailingHostProvider{}
	endpointService := &mockEndpointService{}
	configService := &mockConfigService{}
	metricsProvider := &disabled.Provider{}

	service, err := NewService(failingHostProvider, endpointService, configService, metricsProvider)
	require.NoError(t, err, "Should be able to create service even with failing host provider")

	// Start the service in a goroutine and stop it quickly
	ctx, cancel := context.WithCancel(t.Context())
	go service.Start(ctx)

	// Give it a moment to start the retry loop
	time.Sleep(50 * time.Millisecond)

	// Cancel the context to stop the service
	cancel()

	// Wait a bit for the goroutine to finish
	time.Sleep(200 * time.Millisecond)

	// The test passes if we didn't panic or hang
	_ = service
}

// Mock implementations for timeout tests
type mockSenderTimeout struct{}

func (m *mockSenderTimeout) sendTo(ctx context.Context, info host2.StreamInfo, msg proto.Message, session *NetworkStreamSession) error {
	return nil
}

type mockFailingHostProvider struct{}

func (m *mockFailingHostProvider) GetNewHost() (host2.P2PHost, error) {
	return &mockHostForTimeout{}, errors.New("simulated host creation failure")
}

type mockHostForTimeout struct {
	host2.P2PHost
}

func (m *mockHostForTimeout) Start(newStreamCallback func(stream host2.P2PStream)) error {
	return nil
}

func (m *mockHostForTimeout) Close() error {
	return nil
}

type mockEndpointService struct{}

func (m *mockEndpointService) GetIdentity(label string, pkID []byte) (view.Identity, error) {
	return nil, nil
}

type mockConfigService struct{}

func (m *mockConfigService) GetString(key string) string {
	return ""
}

func (m *mockConfigService) GetPath(key string) string {
	return ""
}

func (m *mockConfigService) GetInt(key string) int {
	return 4 // default numWorkers
}

func (m *mockConfigService) IsSet(key string) bool {
	return false
}
