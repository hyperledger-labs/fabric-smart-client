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

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
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

func (*killableStream) RemotePeerID() host.PeerID             { return "peer1" }
func (*killableStream) RemotePeerAddress() host.PeerIPAddress { return "addr" }

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

// newStreamTestSession returns a session that waits grace for a replacement stream.
func newStreamTestSession(t *testing.T, grace time.Duration) (*P2PNode, *NetworkStreamSession) {
	t.Helper()
	p, err := NewNode(t.Context(), &mockHost{}, &disabled.Provider{})
	require.NoError(t, err)
	session, err := p.getOrCreateSession("sess1", "addr", "ctx", "view", nil, []byte("peer1"), nil)
	require.NoError(t, err)
	session.mutex.Lock()
	session.streamGrace = grace
	session.mutex.Unlock()
	return p, session
}

// waitNoStreams waits until session holds no stream.
func waitNoStreams(t *testing.T, session *NetworkStreamSession) {
	t.Helper()
	require.Eventually(t, func() bool {
		session.mutex.RLock()
		defer session.mutex.RUnlock()
		return len(session.streams) == 0
	}, time.Second, tick, "a dead stream is removed from the session right away")
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
	const grace = 100 * time.Millisecond
	p, session := newStreamTestSession(t, grace)
	attachStream(t, p, session).kill()

	require.Eventually(t, func() bool { return receiveClosed(session) }, grace+time.Second, tick)
	session.mutex.RLock()
	defer session.mutex.RUnlock()
	require.Empty(t, session.streams)
}

// TestSession_PeerChosenMasterIDClosesWhenOnlyStreamDies checks that a session whose peer chose the
// master session's ID still closes when its only stream dies, while the master session does not.
func TestSession_PeerChosenMasterIDClosesWhenOnlyStreamDies(t *testing.T) {
	t.Parallel()
	const grace = 100 * time.Millisecond
	p, err := NewNode(t.Context(), &mockHost{}, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)
	impostor, err := p.getOrCreateSession(masterSession, "addr", "ctx", "view", nil, []byte("peer1"), nil)
	require.NoError(t, err)
	m, err := p.MasterSession()
	require.NoError(t, err)
	master := m.(*NetworkStreamSession)
	for _, s := range []*NetworkStreamSession{impostor, master} {
		s.mutex.Lock()
		s.streamGrace = grace
		s.mutex.Unlock()
		attachStream(t, p, s).kill()
	}

	require.Eventually(t, func() bool { return receiveClosed(impostor) }, grace+time.Second, tick)
	time.Sleep(3 * grace)
	require.False(t, master.Info().Closed)
}

func TestSession_SurvivesWhenReplacementStreamAttaches(t *testing.T) {
	t.Parallel()
	const grace = 100 * time.Millisecond
	p, session := newStreamTestSession(t, grace)
	attachStream(t, p, session).kill()
	waitNoStreams(t, session)

	attachStream(t, p, session)
	time.Sleep(3 * grace)
	require.False(t, session.Info().Closed)
	require.False(t, receiveClosed(session))
}

// TestSession_GraceRestartsWhenLastStreamDiesAgain checks that each loss of the last stream
// gets the full grace period, rather than closing on the timer of an earlier loss.
func TestSession_GraceRestartsWhenLastStreamDiesAgain(t *testing.T) {
	t.Parallel()
	const grace = 2 * time.Second
	p, session := newStreamTestSession(t, grace)
	attachStream(t, p, session).kill()
	waitNoStreams(t, session)

	time.Sleep(grace * 6 / 10)
	attachStream(t, p, session).kill()
	waitNoStreams(t, session)

	// The first loss's grace period has ended, the second one's has not.
	time.Sleep(grace * 6 / 10)
	require.False(t, session.Info().Closed)
	require.Eventually(t, func() bool { return receiveClosed(session) }, grace, tick)
}
