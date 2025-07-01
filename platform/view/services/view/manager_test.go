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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/registry"
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

func TestGetIdentifier(t *testing.T) {
	registry := registry.New()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))
	manager := view2.NewManager(registry, &mock.CommLayer{}, &mock.EndpointService{}, idProvider, view2.NewRegistry(), noop.NewTracerProvider(), &disabled.Provider{}, nil)

	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view_test/DummyView", manager.GetIdentifier(DummyView{}))
	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view_test/DummyView", manager.GetIdentifier(&DummyView{}))
	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view_test/DummyView", manager.GetIdentifier(new(DummyView)))
	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view_test/DummyView", manager.GetIdentifier(*(new(DummyView))))
}

func TestManagerRace(t *testing.T) {
	registry := registry.New()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))
	manager := view2.NewManager(registry, &mock.CommLayer{}, &mock.EndpointService{}, idProvider, view2.NewRegistry(), noop.NewTracerProvider(), &disabled.Provider{}, nil)

	wg := &sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(6)
		go registerFactory(t, wg, manager)
		go newView(t, wg, manager)
		go callView(t, wg, manager)
		go getContext(t, wg, manager)
		go initiateView(t, wg, manager)
		go registerResponder(t, wg, manager)
	}
	wg.Wait()
}

func TestRegisterResponderWithInitiatorView(t *testing.T) {
	registry := registry.New()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))

	manager := view2.NewManager(registry, &mock.CommLayer{}, &mock.EndpointService{}, idProvider, view2.NewRegistry(), noop.NewTracerProvider(), &disabled.Provider{}, nil)
	err := manager.RegisterResponder(&ResponderView{}, &InitiatorView{})
	assert.NoError(t, err)
	responder, _, err := manager.ExistResponderForCaller(manager.GetIdentifier(&InitiatorView{}))
	assert.NoError(t, err)
	res, err := responder.Call(nil)
	assert.NoError(t, err)
	assert.Equal(t, "pineapple", res)

}

func TestRegisterResponderWithViewIdentifier(t *testing.T) {
	registry := registry.New()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))

	manager := view2.NewManager(registry, &mock.CommLayer{}, &mock.EndpointService{}, idProvider, view2.NewRegistry(), noop.NewTracerProvider(), &disabled.Provider{}, nil)
	err := manager.RegisterResponder(&ResponderView{}, manager.GetIdentifier(&InitiatorView{}))
	assert.NoError(t, err)
	responder, _, err := manager.ExistResponderForCaller(manager.GetIdentifier(&InitiatorView{}))
	assert.NoError(t, err)
	res, err := responder.Call(nil)
	assert.NoError(t, err)
	assert.Equal(t, "pineapple", res)
}

func registerFactory(t *testing.T, wg *sync.WaitGroup, m *manager) {
	err := m.RegisterFactory(view2.GenerateUUID(), &DummyFactory{})
	wg.Done()
	assert.NoError(t, err)
}

func registerResponder(t *testing.T, wg *sync.WaitGroup, m *manager) {
	assert.NoError(t, m.RegisterResponderWithIdentity(&DummyView{}, []byte("alice"), &DummyView{}))
	wg.Done()
}

func callView(t *testing.T, wg *sync.WaitGroup, m *manager) {
	_, err := m.InitiateView(context.Background(), &DummyView{})
	wg.Done()
	assert.NoError(t, err)
}

func newView(t *testing.T, wg *sync.WaitGroup, m *manager) {
	_, err := m.NewView(view2.GenerateUUID(), nil)
	wg.Done()
	assert.Error(t, err)
}

func initiateView(t *testing.T, wg *sync.WaitGroup, m *manager) {
	_, err := m.Initiate(context.Background(), view2.GenerateUUID())
	wg.Done()
	assert.Error(t, err)
}

func getContext(t *testing.T, wg *sync.WaitGroup, m *manager) {
	_, err := m.Context("a context")
	wg.Done()
	assert.Error(t, err)
}
