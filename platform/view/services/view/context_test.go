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
	"go.opentelemetry.io/otel/trace/noop"
)

var emptyTracer = noop.NewTracerProvider().Tracer("empty")

type DummyView struct{}

func (d *DummyView) Call(context view.Context) (any, error) {
	return nil, nil
}

type Context interface {
	GetSession(f view.View, party view.Identity, aliases ...view.View) (view.Session, error)
	GetSessionByID(id string, party view.Identity) (view.Session, error)
}

func TestContext(t *testing.T) {
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
	assert.NoError(t, err)

	// Session
	assert.Equal(t, session, ctx.Session())

	// Test NewContext with nil context
	_, err = view2.NewContext(nil, nil, "", nil, nil, nil, nil, nil, nil, nil, nil) //nolint:staticcheck
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "a context should not be nil")

	// Test NewContextForInitiator with nil context
	_, err = view2.NewContextForInitiator("", nil, nil, nil, nil, nil, nil, nil, nil, nil) //nolint:staticcheck
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "a context should not be nil")

	// Test Session with nil session
	ctxNoSession, _ := view2.NewContext(context.TODO(), registry, "p", nil, resolver, idProvider, nil, nil, nil, emptyTracer, nil)
	assert.Nil(t, ctxNoSession.Session())

	// Id
	assert.Equal(t, "pineapple", ctx.ID())

	// Caller
	assert.Equal(t, view.Identity("caller"), ctx.Caller())

	// Identity
	id, err := ctx.Identity("bob")
	assert.NoError(t, err)
	assert.Equal(t, view.Identity("bob"), id)
	arg0, arg1 := resolver.GetIdentityArgsForCall(0)
	assert.Equal(t, "bob", arg0)
	assert.Nil(t, arg1)

	// Me
	assert.Equal(t, view.Identity("charlie"), ctx.Me())

	// Context
	assert.NotNil(t, ctx.Context())

	// OnError / Cleanup
	called := false
	ctx.OnError(func() { called = true })
	ctx.Cleanup()
	assert.True(t, called)

	// PutService / GetService
	err = ctx.PutService("service")
	assert.NoError(t, err)
	s, err := ctx.GetService(reflect.TypeOf(""))
	assert.NoError(t, err)
	assert.Equal(t, "service", s)

	// GetService from registry
	err = registry.RegisterService(123)
	assert.NoError(t, err)
	s2, err := ctx.GetService(reflect.TypeOf(0))
	assert.NoError(t, err)
	assert.Equal(t, 123, s2)

	// IsMe
	lic := &mock.LocalIdentityChecker{}
	ctx, _ = view2.NewContext(context.TODO(), registry, "p", nil, resolver, idProvider, nil, nil, nil, emptyTracer, lic)
	lic.IsMeReturns(true)
	assert.True(t, ctx.IsMe([]byte("me")))

	// StartSpan
	span := ctx.StartSpan("test")
	assert.NotNil(t, span)

	// Initiator
	initiator := &mock.View{}
	ctx, _ = view2.NewContextForInitiator("p2", context.TODO(), registry, nil, resolver, idProvider, nil, initiator, emptyTracer, lic)
	assert.Equal(t, initiator, ctx.Initiator())

	// RunView
	v := &mock.View{}
	v.CallReturns("view-result", nil)
	res, err := ctx.RunView(v)
	assert.NoError(t, err)
	assert.Equal(t, "view-result", res)

	// ResetSessions
	err = ctx.ResetSessions()
	assert.NoError(t, err)

	// PutSession
	session2 := &mock.Session{}
	err = ctx.PutSession(v, []byte("party"), session2)
	assert.NoError(t, err)

	// Dispose
	sessionFactory := &mock.SessionFactory{}
	ctx, _ = view2.NewContext(context.TODO(), registry, "p", sessionFactory, resolver, idProvider, nil, session, nil, emptyTracer, lic)
	ctx.Dispose()
	assert.Equal(t, 2, sessionFactory.DeleteSessionsCallCount())
}

func TestContextGetSession(t *testing.T) {
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
	assert.NoError(t, err)
	assert.NotNil(t, s2)

	// Case: session already exists (reusing s2)
	s, err := ctx.GetSession(dv, party2)
	assert.NoError(t, err)
	assert.Equal(t, session, s)

	// Case: session by ID
	// PutSession only puts it under ViewID + PartyID.
	// GetSessionByID expects it under SessionID + PartyID.
	err = ctx.PutSessionByID("sid", party2, session)
	assert.NoError(t, err)
	s3, err := ctx.GetSessionByID("sid", party2)
	assert.NoError(t, err)
	assert.Equal(t, session, s3)
}

func TestContextRace(t *testing.T) {
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

	ctx, err := view2.NewContext(context.TODO(), registry, "pineapple", sessionFactory, resolver, idProvider, []byte("charlie"), defaultSession, []byte("caller"), emptyTracer, nil)
	assert.NoError(t, err)

	wg := &sync.WaitGroup{}
	dv := &DummyView{}
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go getSession(t, wg, ctx, dv)
		go getSessionByID(t, wg, ctx)
		go getSessionByIDSame(t, wg, ctx)
	}
	wg.Wait()
}

func getSession(t *testing.T, wg *sync.WaitGroup, m Context, dv view.View) {
	_, err := m.GetSession(dv, []byte("alice"))
	wg.Done()
	assert.NoError(t, err)
}

func getSessionByID(t *testing.T, wg *sync.WaitGroup, m Context) {
	_, err := m.GetSessionByID(utils.GenerateUUID(), []byte("alice"))
	wg.Done()
	assert.NoError(t, err)
}

func getSessionByIDSame(t *testing.T, wg *sync.WaitGroup, m Context) {
	_, err := m.GetSessionByID("session id", []byte("alice"))
	wg.Done()
	assert.NoError(t, err)
}
