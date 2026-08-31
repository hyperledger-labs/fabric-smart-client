/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// --- fakes ---

// fakeHostProvider is a configurable GeneratorProvider for testing the Service layer.
type fakeHostProvider struct {
	hostToReturn host.P2PHost
	errToReturn  error
	callCount    atomic.Int32
}

func (f *fakeHostProvider) GetNewHost() (host.P2PHost, error) {
	f.callCount.Add(1)
	return f.hostToReturn, f.errToReturn
}

// --- helpers ---

// newTestService creates a Service wired with fakes that will successfully initialize.
func newTestService(t *testing.T) (*Service, *fakeHostProvider) {
	t.Helper()
	hp := &fakeHostProvider{hostToReturn: &mockHost{}}
	svc, err := NewService(hp, &mockEndpointService{}, &mockConfigService{}, &disabled.Provider{})
	require.NoError(t, err)
	return svc, hp
}

// startAndWait starts the service and waits for initialization to complete.
func startAndWait(t *testing.T, svc *Service) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	svc.Start(ctx)
	require.Eventually(t, func() bool {
		return svc.initialized.Load()
	}, 2*time.Second, 10*time.Millisecond, "service did not initialize in time")
	return cancel
}

// --- tests ---

func TestNewService(t *testing.T) {
	t.Parallel()

	hp := &fakeHostProvider{hostToReturn: &mockHost{}}
	es := &mockEndpointService{}
	cs := &mockConfigService{}
	mp := &disabled.Provider{}

	svc, err := NewService(hp, es, cs, mp)
	require.NoError(t, err)
	require.NotNil(t, svc)
	require.False(t, svc.initialized.Load())
	require.Nil(t, svc.Node)
}

func TestService_BeforeStart(t *testing.T) {
	t.Parallel()

	svc, _ := newTestService(t)

	t.Run("MasterSession returns ErrNotInitialized", func(t *testing.T) {
		t.Parallel()
		_, err := svc.MasterSession()
		require.ErrorIs(t, err, ErrNotInitialized)
	})

	t.Run("NewSessionWithID returns ErrNotInitialized", func(t *testing.T) {
		t.Parallel()
		_, err := svc.NewSessionWithID("s", "c", "e", []byte("p"))
		require.ErrorIs(t, err, ErrNotInitialized)
	})

	t.Run("NewSession returns ErrNotInitialized", func(t *testing.T) {
		t.Parallel()
		_, err := svc.NewSession("caller", "c", "e", []byte("p"))
		require.ErrorIs(t, err, ErrNotInitialized)
	})

	t.Run("NewResponderSession returns ErrNotInitialized", func(t *testing.T) {
		t.Parallel()
		msg := &view.Message{SessionID: "resp"}
		_, err := svc.NewResponderSession(view.Identity("caller"), msg)
		require.ErrorIs(t, err, ErrNotInitialized)
	})

	t.Run("DeleteSessions is a no-op", func(t *testing.T) {
		t.Parallel()
		// Should not panic.
		svc.DeleteSessions(t.Context(), "any")
	})

	t.Run("Stop is a no-op", func(t *testing.T) {
		t.Parallel()
		// Should not panic.
		svc.Stop()
	})
}

func TestService_AfterStart(t *testing.T) {
	t.Parallel()

	svc, hp := newTestService(t)
	cancel := startAndWait(t, svc)
	t.Cleanup(cancel)

	require.True(t, svc.initialized.Load())
	require.Equal(t, int32(1), hp.callCount.Load(), "GetNewHost should have been called exactly once")

	t.Run("MasterSession returns valid session", func(t *testing.T) {
		t.Parallel()
		session, err := svc.MasterSession()
		require.NoError(t, err)
		require.NotNil(t, session)
		require.Equal(t, masterSession, session.Info().ID)
	})

	t.Run("NewSessionWithID returns session with correct properties", func(t *testing.T) {
		t.Parallel()
		session, err := svc.NewSessionWithID("svc-sess", "svc-ctx", "svc-ep", []byte("svc-pk"))
		require.NoError(t, err)
		require.Equal(t, "svc-sess", session.Info().ID)
		require.Equal(t, "svc-ep", session.Info().RemoteEndpoint)
		require.Equal(t, []byte("svc-pk"), session.Info().RemotePKID)
	})

	t.Run("NewSession returns session with random ID", func(t *testing.T) {
		t.Parallel()
		session, err := svc.NewSession("myView", "ctx", "ep", []byte("pk"))
		require.NoError(t, err)
		require.NotEmpty(t, session.Info().ID)
	})

	t.Run("NewResponderSession passes message fields to P2PNode", func(t *testing.T) {
		t.Parallel()
		caller := view.Identity("alice")
		msg := &view.Message{
			SessionID:    "resp-sess-svc",
			ContextID:    "ctx-resp",
			FromEndpoint: "from-ep",
			FromPKID:     []byte("from-pk"),
			Payload:      []byte("payload"),
		}
		session, err := svc.NewResponderSession(caller, msg)
		require.NoError(t, err)

		// Service.NewResponderSession fans the message out into six positional
		// arguments, three of them adjacent strings; assert every one of them.
		info := session.Info()
		require.Equal(t, "resp-sess-svc", info.ID)
		require.Equal(t, caller, info.Caller)
		require.Equal(t, "from-ep", info.RemoteEndpoint)
		require.Equal(t, []byte("from-pk"), info.RemotePKID)
		// A responder session acts on behalf of the remote caller, so it carries
		// no local view ID.
		require.Empty(t, info.CallerViewID)

		// ContextID is not part of view.SessionInfo, so read it off the concrete
		// type to pin the remaining argument.
		ns, ok := session.(*NetworkStreamSession)
		require.True(t, ok)
		ns.mutex.RLock()
		contextID := ns.contextID
		ns.mutex.RUnlock()
		require.Equal(t, "ctx-resp", contextID)
	})

	t.Run("DeleteSessions removes matching sessions", func(t *testing.T) {
		t.Parallel()
		_, err := svc.NewSessionWithID("del-target", "c", "e", []byte("pk-del"))
		require.NoError(t, err)

		svc.DeleteSessions(t.Context(), "del-target")

		svc.NodeSync.RLock()
		node := svc.Node
		svc.NodeSync.RUnlock()
		// Internal keys are "<sessionID>.<hex sha256(pkid)>" (computeInternalSessionID),
		// so build the key rather than guessing at the separator.
		key := computeInternalSessionID("del-target", []byte("pk-del"))
		node.sessionsMutex.Lock()
		_, found := node.sessions[key]
		node.sessionsMutex.Unlock()
		require.False(t, found, "session was not deleted")

		// Creating a new session with the same ID should succeed (the old one was deleted).
		s, err := svc.NewSessionWithID("del-target", "c2", "e2", []byte("pk-del"))
		require.NoError(t, err)
		require.Equal(t, "e2", s.Info().RemoteEndpoint)
	})
}

func TestService_StartWithFailingHostProvider(t *testing.T) {
	t.Parallel()

	hp := &fakeHostProvider{
		hostToReturn: nil,
		errToReturn:  errors.New("host provider broken"),
	}
	svc, err := NewService(hp, &mockEndpointService{}, &mockConfigService{}, &disabled.Provider{})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	svc.Start(ctx)

	require.Eventually(t, func() bool {
		return hp.callCount.Load() >= 1
	}, time.Second, 10*time.Millisecond, "GetNewHost should have been called at least once")
	require.False(t, svc.initialized.Load(), "service must not initialize with a broken host provider")
}
