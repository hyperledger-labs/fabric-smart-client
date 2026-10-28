/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func registered(p *P2PNode, session *NetworkStreamSession) bool {
	p.sessionsMutex.Lock()
	defer p.sessionsMutex.Unlock()
	return p.sessions[computeInternalSessionID(session.sessionID, session.endpointID)] == session
}

func TestSession_UnregisteredWhenOnlyStreamDies(t *testing.T) {
	t.Parallel()
	const grace = 100 * time.Millisecond
	p, session := newStreamTestSession(t, grace)
	attachStream(t, p, session).kill()

	require.Eventually(t, func() bool { return !registered(p, session) }, grace+time.Second, tick)
	require.True(t, session.isClosing.Load())
}

// TestSession_CloseKeepsReplacement checks that closing a session whose registry entry was
// replaced, as disposing the view context that held it does, leaves the replacement registered
// and open.
func TestSession_CloseKeepsReplacement(t *testing.T) {
	t.Parallel()
	p, err := NewNode(t.Context(), &mockHost{}, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)

	old, err := p.getOrCreateSession("s1", "addr", "ctx", "view", nil, []byte("pk"), nil)
	require.NoError(t, err)
	old.closeAsync()
	replacement, err := p.getOrCreateSession("s1", "addr", "ctx", "view", nil, []byte("pk"), nil)
	require.NoError(t, err)
	require.NotSame(t, old, replacement, "a closing session is replaced, not returned")

	old.Close()
	require.True(t, registered(p, replacement))
	require.False(t, replacement.Info().Closed)
}

// TestDispatch_DropsMessagesForSelfClosedSession checks that a message for a session that closed
// on its own is dropped instead of reaching the master session, while a message for a session
// closed by Close reaches it.
func TestDispatch_DropsMessagesForSelfClosedSession(t *testing.T) {
	t.Parallel()

	p, err := NewNode(t.Context(), &mockHost{}, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)
	p.Start(t.Context())
	master, err := p.MasterSession()
	require.NoError(t, err)

	selfClosed, err := p.NewSessionWithID("self", "ctx", "ep", []byte("pk"))
	require.NoError(t, err)
	selfClosed.(*NetworkStreamSession).closeAsync()
	closed, err := p.NewSessionWithID("closed", "ctx", "ep", []byte("pk"))
	require.NoError(t, err)
	closed.Close()

	for _, id := range []string{"self", "closed"} {
		p.incomingMessages <- &messageWithStream{
			message: &view.Message{SessionID: id, FromPKID: []byte("pk"), Payload: []byte(id)},
			stream:  &streamHandler{stream: &mockStream{ctx: t.Context()}, node: p},
		}
	}
	select {
	case msg := <-master.Receive():
		require.Equal(t, "closed", string(msg.Payload))
	case <-time.After(3 * time.Second):
		t.Fatal("message for the closed session did not reach the master session")
	}

	// A new local session with the key of the self-closed one receives its messages again.
	reopened, err := p.NewSessionWithID("self", "ctx", "ep", []byte("pk"))
	require.NoError(t, err)
	p.incomingMessages <- &messageWithStream{
		message: &view.Message{SessionID: "self", FromPKID: []byte("pk"), Payload: []byte("again")},
		stream:  &streamHandler{stream: &mockStream{ctx: t.Context()}, node: p},
	}
	select {
	case msg := <-reopened.Receive():
		require.Equal(t, "again", string(msg.Payload))
	case <-time.After(3 * time.Second):
		t.Fatal("message for the reopened session not delivered")
	}
}

func TestSession_MasterNeverUnregistered(t *testing.T) {
	t.Parallel()
	p, err := NewNode(t.Context(), &mockHost{}, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)
	master, err := p.MasterSession()
	require.NoError(t, err)

	p.unregister(master.(*NetworkStreamSession), true)
	require.True(t, registered(p, master.(*NetworkStreamSession)))
}
