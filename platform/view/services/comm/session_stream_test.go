/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// killableStream blocks reads until kill is called, then returns io.EOF, as a stream does
// when its peer goes away.
type killableStream struct {
	mockStream
	dead     chan struct{}
	killOnce sync.Once
}

func newKillableStream(ctx context.Context) *killableStream {
	return &killableStream{ctx: ctx, dead: make(chan struct{})}
}

func (s *killableStream) Read([]byte) (int, error) {
	<-s.dead
	return 0, io.EOF
}

func (s *killableStream) kill() { s.killOnce.Do(func() { close(s.dead) }) }

func (*killableStream) RemotePeerID() host2.PeerID             { return "peer1" }
func (*killableStream) RemotePeerAddress() host2.PeerIPAddress { return "addr" }

// attachStream opens a stream handler on p and leases it to session, as a send or an
// incoming message does.
func attachStream(t *testing.T, p *P2PNode, session *NetworkStreamSession) *killableStream {
	t.Helper()
	stream := newKillableStream(t.Context())
	t.Cleanup(stream.kill)
	sh := p.handleStream(stream)
	session.mutex.Lock()
	defer session.mutex.Unlock()
	require.True(t, sh.tryLease())
	session.streams[sh] = struct{}{}
	return stream
}

func newStreamTestSession(t *testing.T) (*P2PNode, *NetworkStreamSession) {
	t.Helper()
	p, err := NewNode(t.Context(), &mockHost{}, &disabled.Provider{})
	require.NoError(t, err)
	session, err := p.getOrCreateSession("sess1", "addr", "ctx", "view", nil, []byte("peer1"), nil)
	require.NoError(t, err)
	return p, session
}

func receiveClosed(session *NetworkStreamSession) bool {
	select {
	case _, ok := <-session.Receive():
		return !ok
	default:
		return false
	}
}

func TestSession_ClosedWhenOnlyStreamDies(t *testing.T) {
	t.Parallel()
	p, session := newStreamTestSession(t)
	attachStream(t, p, session).kill()

	require.Eventually(t, func() bool { return receiveClosed(session) }, sessionStreamGrace+100*time.Millisecond, tick)
	session.mutex.RLock()
	defer session.mutex.RUnlock()
	require.Empty(t, session.streams)
}

func TestSession_SurvivesWhenReplacementStreamAttaches(t *testing.T) {
	t.Parallel()
	p, session := newStreamTestSession(t)
	dying := attachStream(t, p, session)
	dying.kill()
	require.Eventually(t, func() bool {
		session.mutex.RLock()
		defer session.mutex.RUnlock()
		return len(session.streams) == 0
	}, time.Second, tick, "a dead stream is removed from the session right away")

	attachStream(t, p, session)
	time.Sleep(sessionStreamGrace + 100*time.Millisecond)
	require.False(t, session.Info().Closed)
	require.False(t, receiveClosed(session))
}
