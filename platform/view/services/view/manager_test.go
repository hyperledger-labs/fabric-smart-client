/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	servicesmock "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestMain(m *testing.M) {
	logging.Init(logging.Config{LogSpec: "debug"})
	m.Run()
}

func TestManager(t *testing.T) {
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
	manager := view.NewManager(ip, registry, metrics, cf)
	assert.NotNil(t, manager)

	// Test Me
	ip.DefaultIdentityReturns(view2.Identity("me"))
	assert.Equal(t, view2.Identity("me"), ip.DefaultIdentity())

	// Test Registry methods through manager
	factory := &mock.Factory{}
	err := manager.RegisterFactory("v1", factory)
	assert.NoError(t, err)

	v := &mock.View{}
	factory.NewViewReturns(v, nil)
	v2, err := manager.NewView("v1", nil)
	assert.NoError(t, err)
	assert.Equal(t, v, v2)

	// Test InitiateView
	ctx := context.Background()
	ip.DefaultIdentityReturns(view2.Identity("me"))
	v.CallReturns("result", nil)

	res, err := manager.InitiateView(ctx, v)
	assert.NoError(t, err)
	assert.Equal(t, "result", res)

	// Test Context
	contexts, err := manager.InitiateContext(ctx, v)
	assert.NoError(t, err)
	assert.NotNil(t, contexts)

	ctxRetrieved, err := manager.Context(contexts.ID())
	assert.NoError(t, err)
	assert.Equal(t, contexts, ctxRetrieved)

	// Test DeleteContext
	manager.DeleteContext(contexts.ID())
	_, err = manager.Context(contexts.ID())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Test InitiateViewWithIdentity
	res, err = manager.InitiateViewWithIdentity(ctx, v, view2.Identity("alice"))
	assert.NoError(t, err)
	assert.Equal(t, "result", res)

	// Test InitiateContextWithIdentity
	c2, err := manager.InitiateContextWithIdentity(ctx, v, view2.Identity("alice"))
	assert.NoError(t, err)
	assert.NotNil(t, c2)

	// Test InitiateContextWithIdentityAndID
	c3, err := manager.InitiateContextWithIdentityAndID(ctx, v, view2.Identity("alice"), "cid3")
	assert.NoError(t, err)
	assert.Equal(t, "cid3", c3.ID())

	// Test GetIdentifier
	assert.NotEmpty(t, manager.GetIdentifier(v))

	// Test GetManager
	sp.GetServiceReturns(manager, nil)
	m2, err := view.GetManager(sp)
	assert.NoError(t, err)
	assert.Equal(t, manager, m2)

	// Test Initiate
	mockCtx := &mock.Context{}
	mockCtx.ContextReturns(context.Background())
	mockCtx.GetServiceReturns(manager, nil)
	res, err = view.Initiate(mockCtx, v)
	assert.NoError(t, err)
	assert.Equal(t, "result", res)

	// Test Manager.Initiate
	err = registry.RegisterResponder(v, "") // Register as initiator
	assert.NoError(t, err)
	res, err = manager.Initiate(context.Background(), view.GetIdentifier(v))
	assert.NoError(t, err)
	assert.Equal(t, "result", res)
}

func TestManagerRegistry(t *testing.T) {
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
	manager := view.NewManager(ip, registry, metrics, cf)

	responder := &mock.View{}
	err := manager.RegisterResponder(responder, "initiator")
	assert.NoError(t, err)

	r, err := manager.GetResponder("initiator")
	assert.NoError(t, err)
	assert.Equal(t, responder, r)

	err = manager.RegisterResponderWithIdentity(responder, view2.Identity("id"), "initiator2")
	assert.NoError(t, err)

	r, id, err := manager.ExistResponderForCaller("initiator2")
	assert.NoError(t, err)
	assert.Equal(t, responder, r)
	assert.Equal(t, view2.Identity("id"), id)
}

func TestNewSessionContext(t *testing.T) {
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
	manager := view.NewManager(ip, registry, metrics, cf)
	ip.DefaultIdentityReturns(view2.Identity("me"))

	session := &mock.Session{}
	session.InfoReturns(view2.SessionInfo{ID: "s1", Caller: view2.Identity("alice")})

	// Case 1: New context
	ctx, isNew, err := manager.NewResponderContext(context.Background(), "c1", session, view2.Identity("alice"), nil)
	assert.NoError(t, err)
	assert.True(t, isNew)
	assert.NotNil(t, ctx)

	// Case 2: Reuse context
	ctx2, isNew, err := manager.NewResponderContext(context.Background(), "c1", session, view2.Identity("alice"), nil)
	assert.NoError(t, err)
	assert.False(t, isNew)
	assert.Equal(t, ctx, ctx2)

	// Case 3: Update session in existing context
	session2 := &mock.Session{}
	session2.InfoReturns(view2.SessionInfo{ID: "s2", Caller: view2.Identity("bob")})
	ctx3, isNew, err := manager.NewResponderContext(context.Background(), "c1", session2, view2.Identity("bob"), nil)
	assert.NoError(t, err)
	assert.False(t, isNew)
	assert.NotEqual(t, ctx, ctx3)
}

func TestManagerOther(t *testing.T) {
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
	manager := view.NewManager(ip, registry, metrics, cf)

	// RegisterContext
	mockCtx := &mock.DisposableContext{}
	mockCtx.IDReturns("mc1")
	mockCtx.ContextReturns(context.Background())
	err := manager.RegisterContext("mc1", mockCtx)
	assert.NoError(t, err)

	c, err := manager.Context("mc1")
	assert.NoError(t, err)
	assert.Equal(t, mockCtx, c)
}
