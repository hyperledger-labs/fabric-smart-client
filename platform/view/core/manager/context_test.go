/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package manager_test

import (
	"context"
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/manager"
	mock2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/manager/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver/mock"
	registry2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/registry"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
)

type Context interface {
	GetSession(f view.View, party view.Identity) (view.Session, error)
	GetSessionByID(id string, party view.Identity) (view.Session, error)
}

func TestContext(t *testing.T) {
	registry := registry2.New()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))
	assert.NoError(t, registry.RegisterService(idProvider))
	assert.NoError(t, registry.RegisterService(&mock2.CommLayer{}))
	resolver := &mock.EndpointService{}
	resolver.GetIdentityReturns([]byte("bob"), nil)
	assert.NoError(t, registry.RegisterService(resolver))
	assert.NoError(t, registry.RegisterService(&mock2.SessionFactory{}))
	session := &mock.Session{}
	ctx, err := manager.NewContext(context.TODO(), registry, "pineapple", nil, driver.GetEndpointService(registry), []byte("charlie"), session, []byte("caller"))
	assert.NoError(t, err)

	// Session
	assert.Equal(t, session, ctx.Session())

	// GetService
	assert.NotNil(t, driver.GetEndpointService(ctx))

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
	registry := registry2.New()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))
	assert.NoError(t, registry.RegisterService(idProvider))
	assert.NoError(t, registry.RegisterService(&mock2.CommLayer{}))
	resolver := &mock.EndpointService{}
	resolver.GetIdentityReturns([]byte("bob"), nil)
	assert.NoError(t, registry.RegisterService(resolver))
	assert.NoError(t, registry.RegisterService(&mock2.SessionFactory{}))
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
	sessionFactory := &mock2.SessionFactory{}
	sessionFactory.NewSessionReturns(session, nil)

	ctx, err := manager.NewContext(context.TODO(), registry, "pineapple", sessionFactory, resolver, []byte("charlie"), defaultSession, []byte("caller"))
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
	_, err := m.GetSessionByID(manager.GenerateUUID(), []byte("alice"))
	wg.Done()
	assert.NoError(t, err)
}

func getSessionByIDSame(t *testing.T, wg *sync.WaitGroup, m Context) {
	_, err := m.GetSessionByID("session id", []byte("alice"))
	wg.Done()
	assert.NoError(t, err)
}
