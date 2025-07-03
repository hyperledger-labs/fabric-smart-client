/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/registry"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
)

var emptyTracer = noop.NewTracerProvider().Tracer("empty")

type Context interface {
	GetSession(f view.View, party view.Identity, aliases ...view.View) (view.Session, error)
	GetSessionByID(id string, party view.Identity) (view.Session, error)
}

func TestContext(t *testing.T) {
	registry := registry.New()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))
	resolver := &mock.EndpointService{}
	resolver.GetIdentityReturns([]byte("bob"), nil)
	session := &mock.Session{}
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
}

func TestContextRace(t *testing.T) {
	registry := registry.New()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))
	resolver := &mock.EndpointService{}
	resolver.ResolverReturns(&endpoint.Resolver{ID: []byte("alice")}, nil, nil)
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
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go getSession(t, wg, ctx)
		go getSessionByID(t, wg, ctx)
		go getSessionByIDSame(t, wg, ctx)
	}
	wg.Wait()
}

func getSession(t *testing.T, wg *sync.WaitGroup, m Context) {
	_, err := m.GetSession(&DummyView{}, []byte("alice"))
	wg.Done()
	assert.NoError(t, err)
}

func getSessionByID(t *testing.T, wg *sync.WaitGroup, m Context) {
	_, err := m.GetSessionByID(view2.GenerateUUID(), []byte("alice"))
	wg.Done()
	assert.NoError(t, err)
}

func getSessionByIDSame(t *testing.T, wg *sync.WaitGroup, m Context) {
	_, err := m.GetSessionByID("session id", []byte("alice"))
	wg.Done()
	assert.NoError(t, err)
}
