/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// --- tests ---

// stopNode tears a test node down completely. P2PNode.Stop() cancels the node
// context, closes the host and closes streams, but it never walks p.sessions --
// closeInternal is reached only from DeleteSession and Session.Close. Without
// closing every session below, every session's tryStart goroutine outlives the
// test, which the package's goleak-guarded tests would eventually trip over.
func stopNode(t *testing.T, p *P2PNode) {
	t.Helper()
	t.Cleanup(func() {
		p.sessionsMutex.Lock()
		for _, session := range p.sessions {
			session.closeInternal()
		}
		clear(p.sessions)
		p.sessionsMutex.Unlock()
		p.Stop()
	})
}

func TestMasterSession(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)

	session, err := p.MasterSession()
	require.NoError(t, err)
	require.NotNil(t, session)
	require.Equal(t, masterSession, session.Info().ID)

	// Calling MasterSession again returns the same session (idempotent).
	session2, err := p.MasterSession()
	require.NoError(t, err)
	require.Same(t, session, session2)
}

func TestNewSessionWithID(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)

	session, err := p.NewSessionWithID("sess-1", "ctx-1", "endpoint-1", []byte("pkid-1"))
	require.NoError(t, err)
	require.NotNil(t, session)
	require.Equal(t, "sess-1", session.Info().ID)
	require.Equal(t, "endpoint-1", session.Info().RemoteEndpoint)
	require.Equal(t, []byte("pkid-1"), session.Info().RemotePKID)
	// Caller is nil when created via NewSessionWithID.
	require.Nil(t, session.Info().Caller)
}

func TestNewResponderSession(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)

	caller := view.Identity("alice")
	msg := &view.Message{
		SessionID: "resp-sess",
		ContextID: "ctx-resp",
		Payload:   []byte("hello"),
	}

	session, err := p.NewResponderSession("resp-sess", "ctx-resp", "endpoint-resp", []byte("pkid-resp"), caller, msg)
	require.NoError(t, err)
	require.NotNil(t, session)
	require.Equal(t, "resp-sess", session.Info().ID)
	require.Equal(t, "endpoint-resp", session.Info().RemoteEndpoint)
	require.Equal(t, caller, session.Info().Caller)

	ch := session.Receive()
	select {
	case receivedMsg := <-ch:
		require.Equal(t, msg.Payload, receivedMsg.Payload)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

// TestReplyError checks that ReplyError neither registers a session nor touches a
// registered one with the same ID, so that a rejection cannot disturb a responder that owns
// or is about to create that session.
func TestReplyError(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)

	owned, err := p.NewResponderSession("sess", "ctx", "ep", []byte("pkid"), view.Identity("alice"), nil)
	require.NoError(t, err)
	sessions := func() int {
		p.sessionsMutex.Lock()
		defer p.sessionsMutex.Unlock()
		return len(p.sessions)
	}
	before := sessions()

	require.NoError(t, p.ReplyError(t.Context(), "sess", "ctx", "ep", []byte("pkid"), []byte("rejected")))
	require.NoError(t, p.ReplyError(t.Context(), "fresh", "ctx", "ep", []byte("pkid"), []byte("rejected")))

	require.Equal(t, before, sessions())
	require.False(t, owned.Info().Closed)
	again, err := p.NewResponderSession("sess", "ctx", "ep", []byte("pkid"), view.Identity("alice"), nil)
	require.NoError(t, err)
	require.Same(t, owned, again)
}

func TestNewSession(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)

	session, err := p.NewSession("myView", "ctx-new", "endpoint-new", []byte("pkid-new"))
	require.NoError(t, err)
	require.NotNil(t, session)
	// NewSession generates a random base64 session ID, so just verify it's non-empty.
	require.NotEmpty(t, session.Info().ID)
	require.Equal(t, "endpoint-new", session.Info().RemoteEndpoint)
	require.Equal(t, []byte("pkid-new"), session.Info().RemotePKID)

	session2, err := p.NewSession("myView", "ctx-new", "endpoint-new", []byte("pkid-new"))
	require.NoError(t, err)
	require.NotEqual(t, session.Info().ID, session2.Info().ID, "session IDs must be unique")
}

func TestGetOrCreateSession_ExistingSessionUpdatesFields(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)

	// Create initial session.
	s1, err := p.NewSessionWithID("sess-update", "ctx-1", "endpoint-1", []byte("pkid"))
	require.NoError(t, err)

	// Re-fetch with updated contextID and endpoint.
	s2, err := p.NewSessionWithID("sess-update", "ctx-2", "endpoint-2", []byte("pkid"))
	require.NoError(t, err)
	// Must be the same session object.
	require.Same(t, s1, s2)
	// Fields must have been updated.
	require.Equal(t, "endpoint-2", s2.Info().RemoteEndpoint)

	require.Empty(t, s2.Info().CallerViewID)
	// contextID is not exposed on view.SessionInfo; read it under the mutex that
	// getOrCreateSession and dispatchMessages write it under.
	require.Equal(t, "ctx-2", contextIDOf(t, s2))
}

// contextIDOf reads a session's contextID under its mutex.
func contextIDOf(t *testing.T, s view.Session) string {
	t.Helper()
	ns, ok := s.(*NetworkStreamSession)
	require.True(t, ok)
	ns.mutex.RLock()
	defer ns.mutex.RUnlock()
	return ns.contextID
}

func TestGetOrCreateSession_CallerMismatchReturnsError(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)

	// Create a session with caller "alice".
	msg := &view.Message{SessionID: "sess-mismatch", Payload: []byte("data")}
	_, err = p.NewResponderSession("sess-mismatch", "ctx", "ep", []byte("pk"), view.Identity("alice"), msg)
	require.NoError(t, err)

	// Re-fetch the same session with a different caller "bob" -- must fail.
	msg2 := &view.Message{SessionID: "sess-mismatch", Payload: []byte("data2")}
	_, err = p.NewResponderSession("sess-mismatch", "ctx-bob", "ep-bob", []byte("pk"), view.Identity("bob"), msg2)
	require.ErrorContains(t, err, "caller identity mismatch")

	// The rejected caller must not have altered the session it failed to claim.
	// Read the stored session directly: re-fetching it through getOrCreateSession
	// would itself rewrite the very fields under test.
	p.sessionsMutex.Lock()
	stored, in := p.sessions[computeInternalSessionID("sess-mismatch", []byte("pk"))]
	p.sessionsMutex.Unlock()
	require.True(t, in)
	require.Equal(t, view.Identity("alice"), stored.Info().Caller)
	require.Equal(t, "ep", stored.Info().RemoteEndpoint)
	require.Equal(t, "ctx", contextIDOf(t, stored))
}

// sessionKeys returns the internal keys of every session registered on p.
func sessionKeys(p *P2PNode) []string {
	p.sessionsMutex.Lock()
	defer p.sessionsMutex.Unlock()
	return slices.Collect(maps.Keys(p.sessions))
}

func TestDeleteSession(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)

	target, err := p.NewSessionWithID("order-1", "ctx", "ep", []byte("pk"))
	require.NoError(t, err)
	// Same ID with another peer, and an ID that extends the target's: neither is the target.
	_, err = p.NewSessionWithID("order-1", "ctx", "ep", []byte("pk-other"))
	require.NoError(t, err)
	_, err = p.NewSessionWithID("order-10", "ctx", "ep", []byte("pk"))
	require.NoError(t, err)

	p.DeleteSession(t.Context(), "order-1", []byte("pk"))

	require.True(t, target.Info().Closed)
	require.ElementsMatch(t, []string{
		computeInternalSessionID("order-1", []byte("pk-other")),
		computeInternalSessionID("order-10", []byte("pk")),
	}, sessionKeys(p))
}

// TestDeleteSession_PeerChosenIDLeavesOtherSessionsOpen covers a remote peer that picks the
// SessionID of its message. The responder session for that message carries the peer's ID and
// PKID, and disposing the responder context deletes it by both. No choice of ID may close the
// master session or a session that belongs to another peer.
func TestDeleteSession_PeerChosenIDLeavesOtherSessionsOpen(t *testing.T) {
	t.Parallel()

	const victimID = "Qk9CLXNlc3Npb24="
	for _, attackerID := range []string{"", "m", masterSession, victimID} {
		t.Run("sessionID="+attackerID, func(t *testing.T) {
			t.Parallel()

			p, err := NewNode(t.Context(), &mockHost{}, &disabled.Provider{})
			require.NoError(t, err)
			stopNode(t, p)

			master, err := p.MasterSession()
			require.NoError(t, err)
			victim, err := p.NewResponderSession(victimID, "ctx-victim", "ep-victim", []byte("pkid-victim"), view.Identity("victim"), nil)
			require.NoError(t, err)

			attackerMsg := &view.Message{SessionID: attackerID, ContextID: "ctx-attacker", Payload: []byte("x")}
			attacker, err := p.NewResponderSession(attackerID, "ctx-attacker", "ep-attacker", []byte("pkid-attacker"), view.Identity("attacker"), attackerMsg)
			require.NoError(t, err)

			// What view.Context.Dispose does when the attacker's responder context is deleted.
			info := attacker.Info()
			p.DeleteSession(t.Context(), info.ID, info.RemotePKID)

			assert.True(t, attacker.Info().Closed)
			assert.False(t, master.Info().Closed, "master session must stay open")
			assert.False(t, victim.Info().Closed, "another peer's session must stay open")
			require.ElementsMatch(t, []string{
				computeInternalSessionID(masterSession, []byte{}),
				computeInternalSessionID(victimID, []byte("pkid-victim")),
			}, sessionKeys(p))
		})
	}
}

func TestDeleteSession_NoMatchIsNoOp(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)

	_, err = p.NewSessionWithID("keep-me", "ctx", "ep", []byte("pk"))
	require.NoError(t, err)

	p.DeleteSession(t.Context(), "keep-me", []byte("other-pk"))
	p.DeleteSession(t.Context(), "nonexistent", []byte("pk"))

	require.Equal(t, []string{computeInternalSessionID("keep-me", []byte("pk"))}, sessionKeys(p))
}
