/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

var emptyTracer = noop.NewTracerProvider().Tracer("empty")

type DummyView struct{}

func (d *DummyView) Call(_ view.Context) (any, error) {
	return nil, nil
}

type Context interface {
	GetSession(f view.View, party view.Identity, aliases ...view.View) (view.Session, error)
	GetSessionByID(id string, party view.Identity) (view.Session, error)
}

func TestContext(t *testing.T) {
	t.Parallel()
	registry := view2.NewServiceProvider()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))
	resolver := &mock.EndpointService{}
	resolver.GetIdentityReturns([]byte("bob"), nil)
	session := &mock.Session{}
	session.InfoReturns(view.SessionInfo{ID: "s1", Caller: view.Identity("caller")})
	ctx, err := view2.NewContext(
		context.TODO(),
		registry,
		"pineapple",
		nil,
		resolver,
		idProvider,
		[]byte("charlie"),
		session,
		[]byte("caller"),
		emptyTracer,
		nil,
	)
	require.NoError(t, err)

	// Session
	require.Equal(t, session, ctx.Session())

	// Test NewContext with nil context
	_, err = view2.NewContext(nil, nil, "", nil, nil, nil, nil, nil, nil, nil, nil) //nolint:staticcheck
	require.Error(t, err)
	require.Contains(t, err.Error(), "ctx should not be nil")

	// Test NewContextForInitiator with nil context
	_, err = view2.NewContextForInitiator("", nil, nil, nil, nil, nil, nil, nil, nil, nil) //nolint:staticcheck
	require.Error(t, err)
	require.Contains(t, err.Error(), "ctx should not be nil")

	// Test Session with nil session
	ctxNoSession, _ := view2.NewContext(context.TODO(), registry, "p", nil, resolver, idProvider, nil, nil, nil, emptyTracer, nil)
	require.Nil(t, ctxNoSession.Session())

	// Id
	require.Equal(t, "pineapple", ctx.ID())

	// Caller
	require.Equal(t, view.Identity("caller"), ctx.Caller())

	// Identity
	id, err := ctx.Identity("bob")
	require.NoError(t, err)
	require.Equal(t, view.Identity("bob"), id)
	arg0, arg1 := resolver.GetIdentityArgsForCall(0)
	require.Equal(t, "bob", arg0)
	require.Nil(t, arg1)

	// Me
	require.Equal(t, view.Identity("charlie"), ctx.Me())

	// Ctx
	require.NotNil(t, ctx.Context())

	// OnError / Cleanup
	called := false
	ctx.OnError(func() { called = true })
	ctx.Cleanup()
	require.True(t, called)

	// PutService / GetService
	err = ctx.PutService("service")
	require.NoError(t, err)
	s, err := ctx.GetService(reflect.TypeOf(""))
	require.NoError(t, err)
	require.Equal(t, "service", s)

	// GetService from registry
	err = registry.RegisterService(123)
	require.NoError(t, err)
	s2, err := ctx.GetService(reflect.TypeOf(0))
	require.NoError(t, err)
	require.Equal(t, 123, s2)

	// IsMe
	lic := &mock.LocalIdentityChecker{}
	ctx, _ = view2.NewContext(context.TODO(), registry, "p", nil, resolver, idProvider, nil, nil, nil, emptyTracer, lic)
	lic.IsMeReturns(true)
	require.True(t, ctx.IsMe([]byte("me")))

	// StartSpan
	span := ctx.StartSpan("test")
	require.NotNil(t, span)

	// Initiator
	initiator := &mock.View{}
	ctx, _ = view2.NewContextForInitiator("p2", context.TODO(), registry, nil, resolver, idProvider, nil, initiator, emptyTracer, lic)
	require.Equal(t, initiator, ctx.Initiator())

	// RunView
	v := &mock.View{}
	v.CallReturns("view-result", nil)
	res, err := ctx.RunView(v)
	require.NoError(t, err)
	require.Equal(t, "view-result", res)

	// ResetSessions
	err = ctx.ResetSessions()
	require.NoError(t, err)

	// PutSession
	session2 := &mock.Session{}
	err = ctx.PutSession(v, []byte("party"), session2)
	require.NoError(t, err)

	// Dispose
	sessionFactory := &mock.SessionFactory{}
	ctx, _ = view2.NewContext(context.TODO(), registry, "p", sessionFactory, resolver, idProvider, nil, session, nil, emptyTracer, lic)
	ctx.Dispose()
	require.Equal(t, 2, sessionFactory.DeleteSessionsCallCount())
}

func TestContextGetSession(t *testing.T) {
	t.Parallel()
	registry := view2.NewServiceProvider()
	idProvider := &mock.IdentityProvider{}
	resolver := &mock.EndpointService{}
	sessionFactory := &mock.SessionFactory{}
	session := &mock.Session{}
	session.InfoReturns(view.SessionInfo{ID: "s1"})

	ctx, _ := view2.NewContext(context.TODO(), registry, "p", sessionFactory, resolver, idProvider, nil, nil, nil, emptyTracer, nil)

	dv := &DummyView{}
	// Case: create new session
	party2 := view.Identity("party2")
	resolver.ResolverReturns(&endpoint.Resolver{ResolverInfo: endpoint.ResolverInfo{ID: party2}}, []byte("pkid"), nil)
	sessionFactory.NewSessionReturns(session, nil)
	s2, err := ctx.GetSession(dv, party2)
	require.NoError(t, err)
	require.NotNil(t, s2)

	// Case: session already exists (reusing s2)
	s, err := ctx.GetSession(dv, party2)
	require.NoError(t, err)
	require.Equal(t, session, s)

	// Case: session by ID
	// PutSession only puts it under ViewID + PartyID.
	// GetSessionByID expects it under SessionID + PartyID.
	err = ctx.PutSessionByID("sid", party2, session)
	require.NoError(t, err)
	s3, err := ctx.GetSessionByID("sid", party2)
	require.NoError(t, err)
	require.Equal(t, session, s3)
}

func TestContextRace(t *testing.T) {
	t.Parallel()
	registry := view2.NewServiceProvider()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))
	resolver := &mock.EndpointService{}
	resolver.ResolverReturns(&endpoint.Resolver{ResolverInfo: endpoint.ResolverInfo{ID: []byte("alice")}}, nil, nil)
	resolver.GetIdentityReturns([]byte("bob"), nil)
	defaultSession := &mock.Session{}
	session := &mock.Session{}
	session.InfoReturns(view.SessionInfo{
		ID:           "",
		Caller:       nil,
		CallerViewID: "",
		Endpoint:     "",
		EndpointPKID: nil,
		Closed:       false,
	})
	sessionFactory := &mock.SessionFactory{}
	sessionFactory.NewSessionReturns(session, nil)

	viewCtx, err := view2.NewContext(context.TODO(), registry, "pineapple", sessionFactory, resolver, idProvider, []byte("charlie"), defaultSession, []byte("caller"), emptyTracer, nil)
	require.NoError(t, err)

	dv := &DummyView{}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Go(func() {
			_, err := viewCtx.GetSession(dv, []byte("alice"))
			assert.NoError(t, err)
		})
		wg.Go(func() {
			_, err := viewCtx.GetSessionByID(utils.GenerateUUID(), []byte("alice"))
			assert.NoError(t, err)
		})
		wg.Go(func() {
			_, err := viewCtx.GetSessionByID("session id", []byte("alice"))
			assert.NoError(t, err)
		})
	}
	wg.Wait()
}
