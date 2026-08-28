/*
Copyright IBM Corp. All Rights Reserved.

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

// --- tests ---

func TestMasterSession(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	t.Cleanup(p.Stop)

	session, err := p.MasterSession()
	require.NoError(t, err)
	require.NotNil(t, session)
	require.Equal(t, masterSession, session.Info().ID)

	// Calling MasterSession again returns the same session (idempotent).
	session2, err := p.MasterSession()
	require.NoError(t, err)
	require.Equal(t, session, session2)
}

func TestNewSessionWithID(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	t.Cleanup(p.Stop)

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
	t.Cleanup(p.Stop)

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
	t.Cleanup(p.Stop)

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
	t.Cleanup(p.Stop)

	// Create initial session.
	s1, err := p.NewSessionWithID("sess-update", "ctx-1", "endpoint-1", []byte("pkid"))
	require.NoError(t, err)

	// Re-fetch with updated contextID and endpoint.
	s2, err := p.NewSessionWithID("sess-update", "ctx-2", "endpoint-2", []byte("pkid"))
	require.NoError(t, err)
	// Must be the same session object.
	require.Equal(t, s1, s2)
	// Fields must have been updated.
	require.Equal(t, "endpoint-2", s2.Info().RemoteEndpoint)

	ns2 := s2.(*NetworkStreamSession)
	require.Equal(t, "ctx-2", ns2.contextID)
	require.Equal(t, "", ns2.callerViewID)
}

func TestGetOrCreateSession_CallerMismatchReturnsError(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	t.Cleanup(p.Stop)

	// Create a session with caller "alice".
	msg := &view.Message{SessionID: "sess-mismatch", Payload: []byte("data")}
	_, err = p.NewResponderSession("sess-mismatch", "ctx", "ep", []byte("pk"), view.Identity("alice"), msg)
	require.NoError(t, err)

	// Re-fetch the same session with a different caller "bob" -- must fail.
	msg2 := &view.Message{SessionID: "sess-mismatch", Payload: []byte("data2")}
	_, err = p.NewResponderSession("sess-mismatch", "ctx", "ep", []byte("pk"), view.Identity("bob"), msg2)
	require.ErrorContains(t, err, "caller identity mismatch")
}

func TestDeleteSessions(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	t.Cleanup(p.Stop)

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

	p.sessionsMutex.Lock()
	countAfter := len(p.sessions)
	p.sessionsMutex.Unlock()
	// Only "payment-1" should remain.
	require.Equal(t, 1, countAfter)
}

func TestDeleteSessions_NoMatchIsNoOp(t *testing.T) {
	t.Parallel()

	h := &mockHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	t.Cleanup(p.Stop)

	_, err = p.NewSessionWithID("keep-me", "ctx", "ep", []byte("pk"))
	require.NoError(t, err)

	p.DeleteSessions(t.Context(), "nonexistent-prefix")

	p.sessionsMutex.Lock()
	count := len(p.sessions)
	p.sessionsMutex.Unlock()
	require.Equal(t, 1, count)
}
