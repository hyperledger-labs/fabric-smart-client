/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

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

func TestNewSessionContext(t *testing.T) {
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
	ip.DefaultIdentityReturns(view2.Identity("me"))

	session := &mock.Session{}
	session.InfoReturns(view2.SessionInfo{ID: "s1", Caller: view2.Identity("alice")})

	// Case 1: New context
	ctx, isNew, err := manager.NewResponderContext(context.Background(), "c1", session, view2.Identity("alice"), nil)
	require.NoError(t, err)
	require.True(t, isNew)
	require.NotNil(t, ctx)

	// Case 2: Reuse context
	ctx2, isNew, err := manager.NewResponderContext(context.Background(), "c1", session, view2.Identity("alice"), nil)
	require.NoError(t, err)
	require.False(t, isNew)
	require.Equal(t, ctx, ctx2)

	// Case 3: Update session in existing context
	session2 := &mock.Session{}
	session2.InfoReturns(view2.SessionInfo{ID: "s2", Caller: view2.Identity("bob")})
	ctx3, isNew, err := manager.NewResponderContext(context.Background(), "c1", session2, view2.Identity("bob"), nil)
	require.NoError(t, err)
	require.False(t, isNew)
	require.NotEqual(t, ctx, ctx3)

	// Case 4: the same session ID from another party is a different session
	session3 := &mock.Session{}
	session3.InfoReturns(view2.SessionInfo{ID: "s2", Caller: view2.Identity("carol"), RemotePKID: []byte("pkid-carol")})
	ctx4, isNew, err := manager.NewResponderContext(context.Background(), "c1", session3, view2.Identity("carol"), nil)
	require.NoError(t, err)
	require.False(t, isNew)
	require.Same(t, session3, ctx4.Session())
}

// TestNewResponderContextRejectsInitiatorContext checks that an incoming session naming the ID
// of a context this node initiated does not get that context. The ID travels on every message
// of the protocol, so any counterparty can name it.
func TestNewResponderContextRejectsInitiatorContext(t *testing.T) {
	t.Parallel()
	sf := &mock.SessionFactory{}
	ip := &mock.IdentityProvider{}
	ip.DefaultIdentityReturns(view2.Identity("me"))
	registry := view.NewRegistry()
	metrics := view.NewMetrics(&disabled.Provider{})
	cf := view.NewContextFactory(&servicesmock.ServiceProvider{}, sf, &mock.EndpointService{}, ip, registry, noop.NewTracerProvider(), metrics, &mock.LocalIdentityChecker{})
	manager := view.NewManager(ip, registry, metrics, cf, view.NewDefaultRunner())

	initiatorCtx, err := manager.InitiateContext(t.Context(), &mock.View{})
	require.NoError(t, err)

	session := &mock.Session{}
	session.InfoReturns(view2.SessionInfo{ID: "s1", Caller: view2.Identity("bob"), RemotePKID: []byte("pkid-bob")})
	responderCtx, isNew, err := manager.NewResponderContext(t.Context(), initiatorCtx.ID(), session, view2.Identity("me"), view2.Identity("bob"))
	require.ErrorContains(t, err, "is not a responder context")
	require.Nil(t, responderCtx)
	require.False(t, isNew)

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
	sf := &mock.SessionFactory{}
	ip := &mock.IdentityProvider{}
	ip.DefaultIdentityReturns(view2.Identity("me"))
	registry := view.NewRegistry()
	metrics := view.NewMetrics(&disabled.Provider{})
	cf := view.NewContextFactory(&servicesmock.ServiceProvider{}, sf, &mock.EndpointService{}, ip, registry, noop.NewTracerProvider(), metrics, &mock.LocalIdentityChecker{})
	manager := view.NewManager(ip, registry, metrics, cf, view.NewDefaultRunner())

	for _, id := range []string{"s1", "s2", "s3"} {
		session := &mock.Session{}
		session.InfoReturns(view2.SessionInfo{ID: id, Caller: view2.Identity("bob"), RemotePKID: []byte("pkid-bob")})
		_, _, err := manager.NewResponderContext(t.Context(), "c1", session, view2.Identity("me"), view2.Identity("bob"))
		require.NoError(t, err)
	}

	manager.DeleteContext("c1")
	deleted := map[string]bool{}
	for i := range sf.DeleteSessionCallCount() {
		_, id, _ := sf.DeleteSessionArgsForCall(i)
		deleted[id] = true
	}
	require.Equal(t, map[string]bool{"s1": true, "s2": true, "s3": true}, deleted)
}

func TestManagerOther(t *testing.T) {
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

	// RegisterContext
	mockCtx := &mock.DisposableContext{}
	mockCtx.IDReturns("mc1")
	mockCtx.ContextReturns(context.Background())
	err := manager.RegisterContext("mc1", mockCtx)
	require.NoError(t, err)

	c, err := manager.Context("mc1")
	require.NoError(t, err)
	require.Equal(t, mockCtx, c)
}
