/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLargeMessages verifies that the system can handle messages up to the 10MB limit.
func TestLargeMessages(t *testing.T) {
	logging.Init(logging.Config{LogSpec: "error"})
	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	defer p.Stop()

	// 5MB Message
	t.Run("5MB Payload", func(t *testing.T) {
		runLargeMessageTest(t, p, 5*1024*1024)
	})

	// 9.9MB Message (To stay within the 10MB total message limit)
	t.Run("9.9MB Payload", func(t *testing.T) {
		runLargeMessageTest(t, p, 9900*1024)
	})
}

func runLargeMessageTest(t *testing.T, p *P2PNode, size int) {
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	session, err := p.getOrCreateSession("large-sess", "addr", "ctx", "view", nil, []byte("large-peer"), nil)
	require.NoError(t, err)
	session.SetEnqueueTimeout(200 * time.Millisecond)

	err = session.Send(payload)
	assert.NoError(t, err)
}

// TestDroppedMessagesMetricValidation verifies that the DroppedMessages metric
// correctly tracks messages lost due to buffer saturation.
func TestDroppedMessagesMetricValidation(t *testing.T) {
	logging.Init(logging.Config{LogSpec: "error"})

	// Create a node with a custom metrics provider to inspect the counter
	metricsProvider := &mockMetricsProvider{counter: &mockCounter{}}
	h := &mockHost{}
	p, err := NewNode(t.Context(), h, metricsProvider)
	require.NoError(t, err)
	defer p.Stop()
	p.Start(t.Context())

	// 1. Test drop in dispatchMessages (master session full)
	masterSess := &NetworkStreamSession{
		node:            p,
		endpointID:      []byte{},
		endpointAddress: "",
		contextID:       "",
		sessionID:       masterSession,
		incoming:        make(chan *view.Message),
		streams:         make(map[*streamHandler]struct{}),
		middleCh:        make(chan *view.Message),
		closing:         make(chan struct{}),
		closed:          make(chan struct{}),
	}
	p.sessionsMutex.Lock()
	p.sessions[computeInternalSessionID(masterSession, []byte{})] = masterSess
	p.sessionsMutex.Unlock()
	// Close it to force drop in dispatchMessages
	masterSess.isClosing.Store(true)
	close(masterSess.closed)

	startDrops := metricsProvider.counter.val.Load()

	// Send a message for an unknown session to trigger dispatcher drop
	p.incomingMessages <- &messageWithStream{
		message: &view.Message{SessionID: "unknown"},
		stream:  &streamHandler{},
	}

	assert.Eventually(t, func() bool {
		return metricsProvider.counter.val.Load() > startDrops
	}, 2*time.Second, 10*time.Millisecond, "DroppedMessages metric did not increment in dispatcher")

	// 2. Test drop in enqueueWithTimeout (session full)
	session := &NetworkStreamSession{
		node:            p,
		endpointID:      []byte("peer2"),
		endpointAddress: "addr",
		contextID:       "ctx",
		sessionID:       "drop-test-2",
		incoming:        make(chan *view.Message), // UNBUFFERED - will block tryStart
		streams:         make(map[*streamHandler]struct{}),
		middleCh:        make(chan *view.Message, 1),
		closing:         make(chan struct{}),
		closed:          make(chan struct{}),
	}
	p.sessionsMutex.Lock()
	p.sessions[computeInternalSessionID("drop-test-2", []byte("peer2"))] = session
	p.sessionsMutex.Unlock()

	// Start tryStart - it will immediately block trying to move messages from middleCh to incoming
	session.tryStart()

	// Fill middleCh (size 1) AND the tryStart's internal state
	session.middleCh <- &view.Message{Payload: []byte("clog-1")}
	// This second send will block session.middleCh since tryStart is blocked on session.incoming
	go func() {
		session.middleCh <- &view.Message{Payload: []byte("clog-2")}
	}()
	// Small wait to ensure goroutine runs
	time.Sleep(50 * time.Millisecond)

	startDrops = metricsProvider.counter.val.Load()
	msg := &view.Message{Payload: []byte("msg")}

	// Now enqueueWithTimeout should block because middleCh is full AND tryStart is blocked on incoming
	success := session.enqueueWithTimeout(msg, 10*time.Millisecond)
	assert.False(t, success)

	assert.Eventually(t, func() bool {
		return metricsProvider.counter.val.Load() > startDrops
	}, 2*time.Second, 10*time.Millisecond, "DroppedMessages metric did not increment in session")
}

// TestConcurrentSendAndClose ensures no panics occur when closing a session
// while multiple goroutines are actively sending.
func TestConcurrentSendAndClose(t *testing.T) {
	logging.Init(logging.Config{LogSpec: "error"})
	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	defer p.Stop()

	session, err := p.getOrCreateSession("stress-sess", "addr", "ctx", "view", nil, []byte("stress-peer"), nil)
	require.NoError(t, err)
	session.SetEnqueueTimeout(200 * time.Millisecond)

	numSenders := 20
	msgsPerSender := 100
	var wg sync.WaitGroup
	wg.Add(numSenders)

	for i := 0; i < numSenders; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < msgsPerSender; j++ {
				_ = session.Send([]byte(fmt.Sprintf("msg-%d-%d", id, j)))
			}
		}(i)
	}

	// Close the session in the middle of the stress
	time.Sleep(10 * time.Millisecond)
	session.Close()

	wg.Wait()
}

// TestRecipientOversizedRejection verifies that the recipient immediately closes
// the connection when receiving a length prefix exceeding the 10MB limit.
func TestRecipientOversizedRejection(t *testing.T) {
	logging.Init(logging.Config{LogSpec: "error"})
	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	defer p.Stop()

	// Create a mock stream that sends an 11MB length prefix
	done := make(chan struct{})
	ms := &mockOversizedStream{
		mockStream: mockStream{ctx: t.Context()},
		onClose:    func() { close(done) },
	}

	// Manually trigger the incoming stream handler
	p.handleIncomingStream(ms)

	select {
	case <-done:
		// Success: the stream was closed by the handler
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout: Recipient did not close the connection after receiving oversized length prefix")
	}
}

type mockOversizedStream struct {
	mockStream
	onClose func()
}

func (m *mockOversizedStream) Read(p []byte) (int, error) {
	// Send a varint for 11MB (11 * 1024 * 1024 = 11534336)
	// Varint for 11534336 is [0x80, 0x80, 0xBF, 0x05]
	oversizedVarint := []byte{0x80, 0x80, 0xBF, 0x05}
	n := copy(p, oversizedVarint)
	return n, nil
}

func (m *mockOversizedStream) Close() error {
	m.onClose()
	return nil
}

type mockMetricsProvider struct {
	disabled.Provider
	counter *mockCounter
}

func (m *mockMetricsProvider) NewCounter(opts metrics.CounterOpts) metrics.Counter {
	if opts.Name == "dropped_messages" {
		return m.counter
	}
	return m.Provider.NewCounter(opts)
}

type mockCounter struct {
	val atomic.Int64
}

func (m *mockCounter) Inc()                                       { m.val.Add(1) }
func (m *mockCounter) Add(delta float64)                          { m.val.Add(int64(delta)) }
func (m *mockCounter) With(labelValues ...string) metrics.Counter { return m }
