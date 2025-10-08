/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockHost struct {
	host2.P2PHost
}

func (m *mockHost) Start(newStreamCallback func(stream host2.P2PStream)) error {
	return nil
}

func (m *mockHost) Close() error {
	return nil
}

type mockStream struct {
	host2.P2PStream
}

func (m *mockStream) Hash() host2.StreamHash            { return "hash" }
func (m *mockStream) Write(p []byte) (n int, err error) { return len(p), nil }
func (m *mockStream) Read(p []byte) (n int, err error) {
	<-context.Background().Done() // Block forever
	return 0, io.EOF
}
func (m *mockStream) Close() error             { return nil }
func (m *mockStream) Context() context.Context { return context.Background() }

func (m *mockHost) NewStream(ctx context.Context, info host2.StreamInfo) (host2.P2PStream, error) {
	return &mockStream{}, nil
}

func (m *mockHost) StreamHash(info host2.StreamInfo) host2.StreamHash {
	return "hash"
}

func TestDispatcherDoS(t *testing.T) {
	logging.Init(logging.Config{
		LogSpec: "fsc.view.services.comm=error",
	})

	smallBuffer := 2
	h := &mockHost{}
	p, err := NewNode(h, &disabled.Provider{})
	require.NoError(t, err)

	// Create sessions MANUALLY to avoid the race in getOrCreateSession -> tryStart
	// we need these sessions to have small buffers
	createSession := func(id string, peerID []byte) *NetworkStreamSession {
		return &NetworkStreamSession{
			node:            p,
			endpointID:      peerID,
			endpointAddress: "addr",
			contextID:       "ctx",
			sessionID:       id,
			callerViewID:    "view",
			incoming:        make(chan *view.Message, smallBuffer),
			streams:         make(map[*streamHandler]struct{}),
			middleCh:        make(chan *view.Message, smallBuffer),
			closing:         make(chan struct{}),
			closed:          make(chan struct{}),
		}
	}

	sessionSlow := createSession("slow", []byte("slow-peer"))
	sessionFast := createSession("fast", []byte("fast-peer"))

	p.sessionsMutex.Lock()
	p.sessions[computeInternalSessionID("slow", []byte("slow-peer"))] = sessionSlow
	p.sessions[computeInternalSessionID("fast", []byte("fast-peer"))] = sessionFast
	p.sessionsMutex.Unlock()

	// NOW start them
	sessionSlow.tryStart()
	sessionFast.tryStart()

	sh := &streamHandler{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	numWorkers := 10
	for i := 0; i < numWorkers; i++ {
		p.dispatchWg.Add(1)
		go p.dispatchMessages(ctx)
	}

	capacity := smallBuffer + 1 + smallBuffer
	for i := 0; i < capacity+1; i++ {
		p.incomingMessages <- &messageWithStream{
			message: &view.Message{
				SessionID: "slow",
				FromPKID:  []byte("slow-peer"),
				Payload:   []byte(fmt.Sprintf("slow-%d", i)),
			},
			stream: sh,
		}
	}

	p.incomingMessages <- &messageWithStream{
		message: &view.Message{
			SessionID: "fast",
			FromPKID:  []byte("fast-peer"),
			Payload:   []byte("fast-msg"),
		},
		stream: sh,
	}

	select {
	case msg := <-sessionFast.Receive():
		assert.Equal(t, "fast-msg", string(msg.Payload))
	case <-time.After(2 * time.Second):
		t.Fatal("TIMED OUT: Fast session blocked by slow session!")
	}
}

func TestMasterSessionDoSProtection(t *testing.T) {
	logging.Init(logging.Config{
		LogSpec: "fsc.view.services.comm=error",
	})

	h := &mockHost{}
	p, err := NewNode(h, &disabled.Provider{})
	require.NoError(t, err)

	// Create a legitimate session
	sessionFast := &NetworkStreamSession{
		node:            p,
		endpointID:      []byte("fast-peer"),
		endpointAddress: "addr",
		contextID:       "ctx",
		sessionID:       "fast",
		callerViewID:    "view",
		incoming:        make(chan *view.Message, 10),
		streams:         make(map[*streamHandler]struct{}),
		middleCh:        make(chan *view.Message, 10),
		closing:         make(chan struct{}),
		closed:          make(chan struct{}),
	}

	// Create a master session with tiny buffer
	masterSess := &NetworkStreamSession{
		node:            p,
		endpointID:      []byte{},
		endpointAddress: "",
		contextID:       "",
		sessionID:       masterSession,
		callerViewID:    "",
		incoming:        make(chan *view.Message, 1),
		streams:         make(map[*streamHandler]struct{}),
		middleCh:        make(chan *view.Message, 1),
		closing:         make(chan struct{}),
		closed:          make(chan struct{}),
	}

	p.sessionsMutex.Lock()
	p.sessions[computeInternalSessionID("fast", []byte("fast-peer"))] = sessionFast
	p.sessions[computeInternalSessionID(masterSession, []byte{})] = masterSess
	p.sessionsMutex.Unlock()

	sessionFast.tryStart()
	masterSess.tryStart()

	// Fill the master session queue
	masterSess.middleCh <- &view.Message{Payload: []byte("clogger-1")}
	masterSess.middleCh <- &view.Message{Payload: []byte("clogger-2")}
	masterSess.middleCh <- &view.Message{Payload: []byte("clogger-3")} // Master is now full (incoming=1, tryStart blocked, middleCh=1)

	sh := &streamHandler{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	numWorkers := 10
	for i := 0; i < numWorkers; i++ {
		p.dispatchWg.Add(1)
		go p.dispatchMessages(ctx)
	}

	// Attacker sends 10 messages for unknown sessions, clogging all 10 workers
	for i := 0; i < numWorkers; i++ {
		p.incomingMessages <- &messageWithStream{
			message: &view.Message{
				SessionID: fmt.Sprintf("unknown-%d", i),
				FromPKID:  []byte("attacker"),
				Payload:   []byte("clog"),
			},
			stream: sh,
		}
	}

	// Now wait a bit for all workers to get stuck on the master session
	// They should each block for 5 seconds.
	time.Sleep(500 * time.Millisecond)

	// SEND a legitimate message.
	start := time.Now()
	p.incomingMessages <- &messageWithStream{
		message: &view.Message{
			SessionID: "fast",
			FromPKID:  []byte("fast-peer"),
			Payload:   []byte("fast-msg"),
		},
		stream: sh,
	}

	select {
	case msg := <-sessionFast.Receive():
		assert.Equal(t, "fast-msg", string(msg.Payload))
		elapsed := time.Since(start)
		// It should take ~4.5 - 5 seconds for a worker to free up and pick this message.
		assert.Greater(t, elapsed, 4*time.Second, "Should have waited for worker to unblock")
		assert.Less(t, elapsed, 10*time.Second, "Should have received message within worker timeout")
	case <-time.After(15 * time.Second):
		t.Fatal("TIMED OUT: Fast session still blocked by clogged master session!")
	}
}

func TestStreamLeak(t *testing.T) {
	logging.Init(logging.Config{
		LogSpec: "fsc.view.services.comm=error",
	})

	h := &mockHost{}
	p, err := NewNode(h, &disabled.Provider{})
	require.NoError(t, err)

	session, err := p.getOrCreateSession("sess1", "addr", "ctx", "view", nil, []byte("peer1"), nil)
	require.NoError(t, err)

	err = session.Send([]byte("hello"))
	require.NoError(t, err)

	p.streamsMutex.RLock()
	require.Len(t, p.streams["hash"], 1)
	sh := p.streams["hash"][0]
	p.streamsMutex.RUnlock()

	session.mutex.RLock()
	_, found := session.streams[sh]
	session.mutex.RUnlock()
	assert.True(t, found, "Session should track the sending stream")
	assert.Equal(t, int64(1), sh.refCtr.Load())

	session.Close()
	assert.Equal(t, int64(0), sh.refCtr.Load(), "refCtr should be 0 after session close")
}
