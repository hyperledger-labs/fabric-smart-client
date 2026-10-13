/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	servicesmock "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestMain(m *testing.M) {
	logging.Init(logging.Config{LogSpec: "debug"})
	m.Run()
}

func TestManager(t *testing.T) {
	t.Parallel()
	sp := &servicesmock.ServiceProvider{}
	sf := &mock.SessionFactory{}
	es := &mock.EndpointService{}
	ip := &mock.IdentityProvider{}
	registry := view.NewRegistry()
	tp := noop.NewTracerProvider()
	mp := &disabled.Provider{}
	lic := &mock.LocalIdentityChecker{}

	metrics := view.NewMetrics(mp)
	cf := view.NewContextFactory(sp, sf, es, ip, registry, tp, metrics, lic)
	manager := view.NewManager(ip, registry, metrics, cf, view.NewDefaultRunner())
	require.NotNil(t, manager)

	// Test Me
	ip.DefaultIdentityReturns(view2.Identity("me"))
	require.Equal(t, view2.Identity("me"), ip.DefaultIdentity())

	// Test Registry methods through manager
	factory := &mock.Factory{}
	err := manager.RegisterFactory("v1", factory)
	require.NoError(t, err)

	v := &mock.View{}
	factory.NewViewReturns(v, nil)
	v2, err := manager.NewView("v1", nil)
	require.NoError(t, err)
	require.Equal(t, v, v2)

	// Test InitiateView
	ctx := context.Background()
	ip.DefaultIdentityReturns(view2.Identity("me"))
	v.CallReturns("result", nil)

	res, err := manager.InitiateView(ctx, v)
	require.NoError(t, err)
	require.Equal(t, "result", res)

	// Test Ctx
	contexts, err := manager.InitiateContext(ctx, v)
	require.NoError(t, err)
	require.NotNil(t, contexts)

	ctxRetrieved, err := manager.Context(contexts.ID())
	require.NoError(t, err)
	require.Equal(t, contexts, ctxRetrieved)

	// Test DeleteContext
	manager.DeleteContext(contexts.ID())
	_, err = manager.Context(contexts.ID())
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")

	// Test InitiateViewWithIdentity
	res, err = manager.InitiateViewWithIdentity(ctx, v, view2.Identity("alice"))
	require.NoError(t, err)
	require.Equal(t, "result", res)

	var nilCtx context.Context
	res, err = manager.InitiateViewWithIdentity(nilCtx, v, view2.Identity("alice"))
	require.Error(t, err)
	require.Nil(t, res)
	require.Contains(t, err.Error(), "context is nil")

	// Test InitiateContextWithIdentity
	c2, err := manager.InitiateContextWithIdentity(ctx, v, view2.Identity("alice"))
	require.NoError(t, err)
	require.NotNil(t, c2)

	// Test InitiateContextWithIdentityAndID
	c3, err := manager.InitiateContextWithIdentityAndID(ctx, v, view2.Identity("alice"), "cid3")
	require.NoError(t, err)
	require.Equal(t, "cid3", c3.ID())

	// Test GetIdentifier
	require.NotEmpty(t, manager.GetIdentifier(v))

	// Test GetManager
	sp.GetServiceReturns(manager, nil)
	m2, err := view.GetManager(sp)
	require.NoError(t, err)
	require.Equal(t, manager, m2)

	// Test Initiate
	mockCtx := &mock.Context{}
	mockCtx.ContextReturns(context.Background())
	mockCtx.GetServiceReturns(manager, nil)
	res, err = view.Initiate(mockCtx, v)
	require.NoError(t, err)
	require.Equal(t, "result", res)

	// Test Manager.Initiate
	err = registry.RegisterResponder(v, "") // Register as initiator
	require.NoError(t, err)
	res, err = manager.Initiate(context.Background(), view.GetIdentifier(v))
	require.NoError(t, err)
	require.Equal(t, "result", res)
}

func TestManagerRegistry(t *testing.T) {
	t.Parallel()
	sp := &servicesmock.ServiceProvider{}
	sf := &mock.SessionFactory{}
	es := &mock.EndpointService{}
	ip := &mock.IdentityProvider{}
	registry := view.NewRegistry()
	tp := noop.NewTracerProvider()
	mp := &disabled.Provider{}
	lic := &mock.LocalIdentityChecker{}

	metrics := view.NewMetrics(mp)
	cf := view.NewContextFactory(sp, sf, es, ip, registry, tp, metrics, lic)
	manager := view.NewManager(ip, registry, metrics, cf, view.NewDefaultRunner())

	responder := &mock.View{}
	err := manager.RegisterResponder(responder, "initiator")
	require.NoError(t, err)

	r, err := manager.GetResponder("initiator")
	require.NoError(t, err)
	require.Equal(t, responder, r)

	err = manager.RegisterResponderWithIdentity(responder, view2.Identity("id"), "initiator2")
	require.NoError(t, err)

	r, id, err := manager.ExistResponderForCaller("initiator2")
	require.NoError(t, err)
	require.Equal(t, responder, r)
	require.Equal(t, view2.Identity("id"), id)
}

// newResponderManager returns a Manager backed by a real context factory, and the session
// factory its contexts delete sessions with.
func newResponderManager() (*view.Manager, *mock.SessionFactory) {
	sf := &mock.SessionFactory{}
	ip := &mock.IdentityProvider{}
	ip.DefaultIdentityReturns(view2.Identity("me"))
	registry := view.NewRegistry()
	metrics := view.NewMetrics(&disabled.Provider{})
	cf := view.NewContextFactory(&servicesmock.ServiceProvider{}, sf, &mock.EndpointService{}, ip, registry, noop.NewTracerProvider(), metrics, &mock.LocalIdentityChecker{})
	return view.NewManager(ip, registry, metrics, cf, view.NewDefaultRunner()), sf
}

func newSession(id, caller, pkid string) *mock.Session {
	s := &mock.Session{}
	s.InfoReturns(view2.SessionInfo{ID: id, Caller: view2.Identity(caller), RemotePKID: []byte(pkid)})
	return s
}

// deletedSessions returns the sessions deleted through sf, as "id@pkid".
func deletedSessions(sf *mock.SessionFactory) map[string]bool {
	deleted := map[string]bool{}
	for i := range sf.DeleteSessionCallCount() {
		_, id, pkid := sf.DeleteSessionArgsForCall(i)
		deleted[id+"@"+string(pkid)] = true
	}
	return deleted
}

func TestNewSessionContext(t *testing.T) {
	t.Parallel()
	manager, _ := newResponderManager()

	// Case 1: New context
	session := newSession("s1", "alice", "")
	ctx, _, err := manager.NewResponderContext(t.Context(), "c1", session, view2.Identity("alice"), nil)
	require.NoError(t, err)
	require.NotNil(t, ctx)
	registered, err := manager.Context("c1")
	require.NoError(t, err)
	require.Same(t, session, registered.Session())

	// Case 2: Reuse context, the same session joins without replacing the registered context
	ctx2, _, err := manager.NewResponderContext(t.Context(), "c1", session, view2.Identity("alice"), nil)
	require.NoError(t, err)
	require.Same(t, session, ctx2.Session())
	again, err := manager.Context("c1")
	require.NoError(t, err)
	require.Same(t, registered, again)

	// Case 3: Update session in existing context
	session2 := newSession("s2", "bob", "")
	ctx3, _, err := manager.NewResponderContext(t.Context(), "c1", session2, view2.Identity("bob"), nil)
	require.NoError(t, err)
	require.Same(t, session2, ctx3.Session())
	registered, err = manager.Context("c1")
	require.NoError(t, err)
	require.Same(t, session2, registered.Session())

	// Case 4: the same session ID from another party is a different session
	session3 := newSession("s2", "carol", "pkid-carol")
	ctx4, _, err := manager.NewResponderContext(t.Context(), "c1", session3, view2.Identity("carol"), nil)
	require.NoError(t, err)
	require.Same(t, session3, ctx4.Session())
}

// TestNewResponderContextRejectsInitiatorContext checks that an incoming session naming the ID
// of a context this node initiated does not get that context. The ID travels on every message
// of the protocol, so any counterparty can name it.
func TestNewResponderContextRejectsInitiatorContext(t *testing.T) {
	t.Parallel()
	manager, _ := newResponderManager()

	initiatorCtx, err := manager.InitiateContext(t.Context(), &mock.View{})
	require.NoError(t, err)

	responderCtx, release, err := manager.NewResponderContext(t.Context(), initiatorCtx.ID(), newSession("s1", "bob", "pkid-bob"), view2.Identity("me"), view2.Identity("bob"))
	require.ErrorIs(t, err, view.ErrNotResponderContext)
	require.Nil(t, responderCtx)
	require.Nil(t, release)

	// The initiator context is still registered, untouched.
	c, err := manager.Context(initiatorCtx.ID())
	require.NoError(t, err)
	require.Same(t, initiatorCtx, c)
	require.Nil(t, c.Session())
}

// TestNewResponderContextAttachedSessionsAreDisposed checks that every session attached to a
// responder context, including several from the same party, is deleted when the context is.
func TestNewResponderContextAttachedSessionsAreDisposed(t *testing.T) {
	t.Parallel()
	manager, sf := newResponderManager()

	// The same party attaches several times, also with one session ID from two of its PKIDs.
	for _, s := range []*mock.Session{
		newSession("s1", "bob", "pkid-bob-1"),
		newSession("s2", "bob", "pkid-bob-1"),
		newSession("s3", "bob", "pkid-bob-1"),
		newSession("s3", "bob", "pkid-bob-2"),
	} {
		_, _, err := manager.NewResponderContext(t.Context(), "c1", s, view2.Identity("me"), view2.Identity("bob"))
		require.NoError(t, err)
	}

	manager.DeleteContext("c1")
	require.Equal(t, map[string]bool{"s1@pkid-bob-1": true, "s2@pkid-bob-1": true, "s3@pkid-bob-1": true, "s3@pkid-bob-2": true}, deletedSessions(sf))
}

// TestNewResponderContextAttachKeepsDefaultSession checks that the peer-chosen ID of an attaching
// session cannot replace the default session, which responders reach with GetSession(nil, party).
func TestNewResponderContextAttachKeepsDefaultSession(t *testing.T) {
	t.Parallel()
	manager, _ := newResponderManager()
	bob := view2.Identity("bob")

	s1 := newSession("s1", "bob", "pkid-bob")
	ctx, _, err := manager.NewResponderContext(t.Context(), "c1", s1, view2.Identity("me"), bob)
	require.NoError(t, err)

	_, _, err = manager.NewResponderContext(t.Context(), "c1", newSession("", "bob", "pkid-bob"), view2.Identity("me"), bob)
	require.ErrorIs(t, err, view.ErrInvalidSessionID)

	_, _, err = manager.NewResponderContext(t.Context(), "c1", newSession("s2", "bob", "pkid-bob"), view2.Identity("me"), bob)
	require.NoError(t, err)

	s, err := ctx.GetSession(nil, bob)
	require.NoError(t, err)
	require.Same(t, s1, s)
}

// TestNewResponderContextLastReleaseDisposes checks that a responder context, shared by the
// responder that created it and one that attached later, is disposed only when the last of
// them releases it, whichever returns first.
func TestNewResponderContextLastReleaseDisposes(t *testing.T) {
	t.Parallel()
	for _, creatorFirst := range []bool{true, false} {
		manager, sf := newResponderManager()
		creatorCtx, cancelCreator := context.WithCancel(t.Context())
		creator, releaseCreator, err := manager.NewResponderContext(creatorCtx, "c1", newSession("s1", "bob", "pkid-bob"), view2.Identity("me"), view2.Identity("bob"))
		require.NoError(t, err)
		attached, releaseAttached, err := manager.NewResponderContext(t.Context(), "c1", newSession("s2", "bob", "pkid-bob"), view2.Identity("me"), view2.Identity("bob"))
		require.NoError(t, err)

		first, last := releaseCreator, releaseAttached
		if !creatorFirst {
			first, last = releaseAttached, releaseCreator
		}
		first()
		first() // a second call is a no-op
		if creatorFirst {
			// respond cancels the creator's go context when the creator returns.
			cancelCreator()
			require.Error(t, creator.Context().Err(), "each responder runs on its own go context")
			require.NoError(t, attached.Context().Err(), "the attached responder keeps its own go context")
		}
		require.Equal(t, 0, sf.DeleteSessionCallCount(), "no session is deleted while a responder runs")
		c, err := manager.Context("c1")
		require.NoError(t, err)
		require.NoError(t, c.Context().Err(), "the shared go context outlives each responder's")

		last()
		require.Equal(t, map[string]bool{"s1@pkid-bob": true, "s2@pkid-bob": true}, deletedSessions(sf))
		require.Error(t, c.Context().Err(), "the shared go context is cancelled at the last release")
		_, err = manager.Context("c1")
		require.ErrorIs(t, err, view.ErrContextNotFound)
		cancelCreator()
	}
}

// TestNewResponderContextReleasedWhenCtxDone checks that a responder whose go context is done
// releases its context.
func TestNewResponderContextReleasedWhenCtxDone(t *testing.T) {
	t.Parallel()
	manager, sf := newResponderManager()

	ctx, cancel := context.WithCancel(t.Context())
	_, _, err := manager.NewResponderContext(ctx, "c1", newSession("s1", "bob", "pkid-bob"), view2.Identity("me"), view2.Identity("bob"))
	require.NoError(t, err)
	cancel()
	require.Eventually(t, func() bool {
		_, err := manager.Context("c1")
		return errors.Is(err, view.ErrContextNotFound)
	}, time.Second, time.Millisecond)
	require.Equal(t, map[string]bool{"s1@pkid-bob": true}, deletedSessions(sf))
}

// TestNewResponderContextReleaseAfterDelete checks that releasing a context that DeleteContext
// removed neither disposes it again nor touches a newer context with the same ID.
func TestNewResponderContextReleaseAfterDelete(t *testing.T) {
	t.Parallel()
	manager, sf := newResponderManager()

	_, release, err := manager.NewResponderContext(t.Context(), "c1", newSession("s1", "bob", "pkid-bob"), view2.Identity("me"), view2.Identity("bob"))
	require.NoError(t, err)
	manager.DeleteContext("c1")
	deletes := sf.DeleteSessionCallCount()

	_, _, err = manager.NewResponderContext(t.Context(), "c1", newSession("s2", "bob", "pkid-bob"), view2.Identity("me"), view2.Identity("bob"))
	require.NoError(t, err)
	release()
	c, err := manager.Context("c1")
	require.NoError(t, err)
	require.Equal(t, "s2", c.Session().Info().ID)
	require.Equal(t, deletes, sf.DeleteSessionCallCount())
}
