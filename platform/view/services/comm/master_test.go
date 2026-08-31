/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// --- tests ---

// stopNode tears a test node down completely. P2PNode.Stop() cancels the node
// context, closes the host and closes streams, but it never walks p.sessions --
// closeInternal is reached only from DeleteSessions and Session.Close. Without
// the DeleteSessions call below, every session's tryStart goroutine outlives the
// test, which the package's goleak-guarded tests would eventually trip over.
func stopNode(t *testing.T, p *P2PNode) {
	t.Helper()
	t.Cleanup(func() {
		p.DeleteSessions(context.Background(), "")
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

func TestDeleteSessions(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)

	// Create two sessions with a shared prefix and one unrelated session.
	_, err = p.NewSessionWithID("order-1", "ctx", "ep", []byte("pk"))
	require.NoError(t, err)
	_, err = p.NewSessionWithID("order-2", "ctx", "ep", []byte("pk"))
	require.NoError(t, err)
	_, err = p.NewSessionWithID("payment-1", "ctx", "ep", []byte("pk"))
	require.NoError(t, err)

	p.sessionsMutex.Lock()
	countBefore := len(p.sessions)
	p.sessionsMutex.Unlock()
	require.Equal(t, 3, countBefore)

	// Delete sessions whose internal key starts with "order-".
	p.DeleteSessions(t.Context(), "order-")

	// Only "payment-1" should remain: assert the surviving key, not just the
	// count -- a predicate that also deleted payment-1 and kept an order-* key
	// would still leave exactly one session behind.
	p.sessionsMutex.Lock()
	remaining := make([]string, 0, len(p.sessions))
	for key := range p.sessions {
		remaining = append(remaining, key)
	}
	p.sessionsMutex.Unlock()
	require.Equal(t, []string{computeInternalSessionID("payment-1", []byte("pk"))}, remaining)
}

func TestDeleteSessions_NoMatchIsNoOp(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)

	_, err = p.NewSessionWithID("keep-me", "ctx", "ep", []byte("pk"))
	require.NoError(t, err)

	p.DeleteSessions(t.Context(), "nonexistent-prefix")

	p.sessionsMutex.Lock()
	count := len(p.sessions)
	p.sessionsMutex.Unlock()
	require.Equal(t, 1, count)
}
