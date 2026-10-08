/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestEnqueueFullQueue(t *testing.T) { //nolint:paralleltest
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// enqueue drops a message for a full queue without blocking, and leaves closing to the dispatcher.
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

	start := time.Now()
	require.False(t, s.enqueue(&view.Message{Payload: []byte("msg2")}), "Enqueue on a full queue should fail")
	require.Less(t, time.Since(start), 500*time.Millisecond, "Enqueue should not block")
	require.False(t, s.isClosed(), "Enqueue should not close the session")

	sess.Close()
	require.False(t, s.enqueue(&view.Message{Payload: []byte("msg3")}), "Enqueue on closed session should return false")
}

func TestCloseAsyncRefusesLaterMessages(t *testing.T) { //nolint:paralleltest
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	s := &NetworkStreamSession{
		node:     &mockSenderTimeout{},
		incoming: make(chan *view.Message, 1),
		streams:  make(map[*streamHandler]struct{}),
		middleCh: make(chan *view.Message, 1),
		closing:  make(chan struct{}),
		closed:   make(chan struct{}),
	}

	// The queue has room, so only the closing mark can refuse the message.
	s.closeAsync()
	require.False(t, s.enqueue(&view.Message{Payload: []byte("msg")}), "Enqueue after closeAsync should fail")
	require.Eventually(t, s.isClosed, time.Second, time.Millisecond)
}

func TestCloseWaitsForCloseAsync(t *testing.T) { //nolint:paralleltest
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	p, err := NewNode(context.Background(), &mockHost{}, &disabled.Provider{})
	require.NoError(t, err)
	s := &NetworkStreamSession{
		node:     p,
		incoming: make(chan *view.Message), // unbuffered, never read
		streams:  make(map[*streamHandler]struct{}),
		middleCh: make(chan *view.Message, 1),
		closing:  make(chan struct{}),
		closed:   make(chan struct{}),
	}
	sh := &streamHandler{stream: &mockStream{ctx: t.Context()}, node: p}
	require.True(t, sh.tryLease())
	s.streams[sh] = struct{}{}

	// Undelivered messages keep the background close draining.
	require.True(t, s.enqueue(&view.Message{Payload: []byte("msg1")}))
	require.Eventually(t, func() bool { return len(s.middleCh) == 0 }, time.Second, time.Millisecond)
	require.True(t, s.enqueue(&view.Message{Payload: []byte("msg2")}))
	s.closeAsync()
	s.Close()

	require.True(t, s.isClosed())
	assert.Zero(t, sh.refCtr.Load(), "Close returned before the streams were released")
}

func TestDrainTimeoutOnClose(t *testing.T) { //nolint:paralleltest
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

func TestServiceStartRetryDelay(t *testing.T) { //nolint:paralleltest
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

func (*mockSenderTimeout) sendTo(_ context.Context, _ host2.StreamInfo, _ proto.Message, _ *NetworkStreamSession) error {
	return nil
}

type mockFailingHostProvider struct{}

func (*mockFailingHostProvider) GetNewHost() (host2.P2PHost, error) {
	return &mockHostForTimeout{}, errors.New("simulated host creation failure")
}

type mockHostForTimeout struct {
	host2.P2PHost
}

func (*mockHostForTimeout) PeerID() host2.PeerID {
	return "mock-peer"
}

func (*mockHostForTimeout) Start(_ func(stream host2.P2PStream)) error {
	return nil
}

func (*mockHostForTimeout) Close() error {
	return nil
}

type mockEndpointService struct{}

func (*mockEndpointService) GetIdentity(_ string, _ []byte) (view.Identity, error) {
	return nil, nil
}

type mockConfigService struct{}

func (*mockConfigService) GetString(_ string) string {
	return ""
}

func (*mockConfigService) GetPath(_ string) string {
	return ""
}

func (*mockConfigService) GetInt(_ string) int {
	return 4 // default numWorkers
}

func (*mockConfigService) IsSet(_ string) bool {
	return false
}
