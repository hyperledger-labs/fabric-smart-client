/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
)

type manager = view2.Manager

type InitiatorView struct{}

func (a InitiatorView) Call(context view.Context) (any, error) {
	return nil, nil
}

type ResponderView struct{}

func (a ResponderView) Call(context view.Context) (any, error) {
	return "pineapple", nil
}

type DummyView struct{}

func (a DummyView) Call(context view.Context) (any, error) {
	time.Sleep(2 * time.Second)
	return nil, nil
}

type DummyFactory struct{}

func (d *DummyFactory) NewView(in []byte) (view.View, error) {
	time.Sleep(2 * time.Second)
	return nil, nil
}

func (d *DummyFactory) NewViewWithArg(in any) (view.View, error) {
	time.Sleep(2 * time.Second)
	return nil, nil
}

func TestGetIdentifier(t *testing.T) {
	viewRegistry := view2.NewRegistry()
	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view_test/DummyView", viewRegistry.GetIdentifier(DummyView{}))
	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view_test/DummyView", viewRegistry.GetIdentifier(&DummyView{}))
	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view_test/DummyView", viewRegistry.GetIdentifier(new(DummyView)))
	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view_test/DummyView", viewRegistry.GetIdentifier(*(new(DummyView))))
}

func TestManagerRace(t *testing.T) {
	registry := view2.NewServiceProvider()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))

	v := make(<-chan *view.Message)

	session := &mock.Session{}
	session.ReceiveReturns(v)

	commLayer := mock.CommLayer{}
	commLayer.MasterSessionReturns(session, nil)

	manager := view2.NewManager(registry, &commLayer, &mock.EndpointService{}, idProvider, view2.NewRegistry(), noop.NewTracerProvider(), &disabled.Provider{}, nil)

	ctx, cancelFunc := context.WithCancel(context.Background())
	go func() {
		t.Logf("context cancelled")
		time.Sleep(1 * time.Second)
		cancelFunc()
	}()

	wg := &sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(8)
		go registerFactory(t, wg, manager)
		go registerLocalFactory(t, wg, manager)
		go newView(t, wg, manager)
		go newLocalView(t, wg, manager)
		go callView(t, wg, manager)
		go getContext(t, wg, manager)
		go initiateView(t, wg, manager)
		go start(t, wg, manager, ctx)
		go registerResponder(t, wg, manager)
	}
	wg.Wait()
}

func TestRegisterResponderWithInitiatorView(t *testing.T) {
	registry := view2.NewServiceProvider()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))

	manager := view2.NewManager(registry, &mock.CommLayer{}, &mock.EndpointService{}, idProvider, view2.NewRegistry(), noop.NewTracerProvider(), &disabled.Provider{}, nil)
	err := manager.Registry().RegisterResponder(&ResponderView{}, &InitiatorView{})
	assert.NoError(t, err)
	responder, _, err := manager.Registry().ExistResponderForCaller(manager.Registry().GetIdentifier(&InitiatorView{}))
	assert.NoError(t, err)
	res, err := responder.Call(nil)
	assert.NoError(t, err)
	assert.Equal(t, "pineapple", res)

}

func TestRegisterResponderWithViewIdentifier(t *testing.T) {
	registry := view2.NewServiceProvider()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))

	manager := view2.NewManager(registry, &mock.CommLayer{}, &mock.EndpointService{}, idProvider, view2.NewRegistry(), noop.NewTracerProvider(), &disabled.Provider{}, nil)
	err := manager.Registry().RegisterResponder(&ResponderView{}, manager.Registry().GetIdentifier(&InitiatorView{}))
	assert.NoError(t, err)
	responder, _, err := manager.Registry().ExistResponderForCaller(manager.Registry().GetIdentifier(&InitiatorView{}))
	assert.NoError(t, err)
	res, err := responder.Call(nil)
	assert.NoError(t, err)
	assert.Equal(t, "pineapple", res)
}

func registerFactory(t *testing.T, wg *sync.WaitGroup, m *manager) {
	err := m.Registry().RegisterFactory(view2.GenerateUUID(), &DummyFactory{})
	wg.Done()
	assert.NoError(t, err)
}

func registerLocalFactory(t *testing.T, wg *sync.WaitGroup, m *manager) {
	err := m.Registry().RegisterLocalFactory(view2.GenerateUUID(), &DummyFactory{})
	wg.Done()
	assert.NoError(t, err)
}

func registerResponder(t *testing.T, wg *sync.WaitGroup, m *manager) {
	assert.NoError(t, m.Registry().RegisterResponderWithIdentity(&DummyView{}, []byte("alice"), &DummyView{}))
	wg.Done()
}

func callView(t *testing.T, wg *sync.WaitGroup, m *manager) {
	_, err := m.InitiateView(context.Background(), &DummyView{})
	wg.Done()
	assert.NoError(t, err)
}

func newView(t *testing.T, wg *sync.WaitGroup, m *manager) {
	_, err := m.Registry().NewView(view2.GenerateUUID(), nil)
	wg.Done()
	assert.Error(t, err)
}

func newLocalView(t *testing.T, wg *sync.WaitGroup, m *manager) {
	_, err := m.Registry().NewLocalView(view2.GenerateUUID(), nil)
	wg.Done()
	assert.Error(t, err)
}

func initiateView(t *testing.T, wg *sync.WaitGroup, m *manager) {
	_, err := m.Initiate(context.Background(), view2.GenerateUUID())
	wg.Done()
	assert.Error(t, err)
}

func getContext(t *testing.T, wg *sync.WaitGroup, m *manager) {
	_, err := m.ContextByID("a context")
	wg.Done()
	assert.Error(t, err)
}

func start(t *testing.T, wg *sync.WaitGroup, m manager, ctx context.Context) {
	m.Start(ctx)
	wg.Done()
}
