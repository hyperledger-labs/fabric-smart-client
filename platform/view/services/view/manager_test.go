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

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
)

type Manager interface {
	InitiateView(f view.View, ctx context.Context) (interface{}, error)
	Context(id string) (view.Context, error)
	RegisterFactory(id string, factory view2.Factory) error
	NewView(id string, in []byte) (f view.View, err error)
	Initiate(id string, ctx context.Context) (interface{}, error)
	RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy interface{}) error
	Start(ctx context.Context)
}

type InitiatorView struct{}

func (a InitiatorView) Call(context view.Context) (interface{}, error) {
	return nil, nil
}

type ResponderView struct{}

func (a ResponderView) Call(context view.Context) (interface{}, error) {
	return "pineapple", nil
}

type DummyView struct{}

func (a DummyView) Call(context view.Context) (interface{}, error) {
	time.Sleep(2 * time.Second)
	return nil, nil
}

type ContextKey string

type DummyViewContextCheck struct{}

func (a DummyViewContextCheck) Call(ctx view.Context) (interface{}, error) {
	v, ok := ctx.Context().Value(ContextKey("test")).(string)
	if !ok {
		return nil, errors.Errorf("context value %s not found", ContextKey("test"))
	}
	return v, nil
}

type DummyFactory struct{}

func (d *DummyFactory) NewView(in []byte) (view.View, error) {
	time.Sleep(2 * time.Second)
	return nil, nil
}

func TestGetIdentifier(t *testing.T) {
	registry := view2.NewServiceProvider()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))
	manager := view2.NewManager(registry, &mock.CommLayer{}, &mock.EndpointService{}, idProvider, view2.NewRegistry(), noop.NewTracerProvider(), &disabled.Provider{}, nil)

	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view_test/DummyView", manager.GetIdentifier(DummyView{}))
	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view_test/DummyView", manager.GetIdentifier(&DummyView{}))
	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view_test/DummyView", manager.GetIdentifier(new(DummyView)))
	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view_test/DummyView", manager.GetIdentifier(*(new(DummyView))))
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
		go newView(t, wg, manager)
		go callView(t, wg, manager)
		go callViewWithAugmentedContext(t, wg, manager)
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
	err := manager.RegisterResponder(&ResponderView{}, &InitiatorView{})
	assert.NoError(t, err)
	responder, _, err := manager.ExistResponderForCaller(manager.GetIdentifier(&InitiatorView{}))
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
	err := manager.RegisterResponder(&ResponderView{}, manager.GetIdentifier(&InitiatorView{}))
	assert.NoError(t, err)
	responder, _, err := manager.ExistResponderForCaller(manager.GetIdentifier(&InitiatorView{}))
	assert.NoError(t, err)
	res, err := responder.Call(nil)
	assert.NoError(t, err)
	assert.Equal(t, "pineapple", res)
}

func registerFactory(t *testing.T, wg *sync.WaitGroup, m Manager) {
	err := m.RegisterFactory(utils.GenerateUUID(), &DummyFactory{})
	wg.Done()
	assert.NoError(t, err)
}

func registerResponder(t *testing.T, wg *sync.WaitGroup, m Manager) {
	assert.NoError(t, m.RegisterResponderWithIdentity(&DummyView{}, []byte("alice"), &DummyView{}))
	wg.Done()
}

func callView(t *testing.T, wg *sync.WaitGroup, m Manager) {
	_, err := m.InitiateView(&DummyView{}, context.Background())
	wg.Done()
	assert.NoError(t, err)
}

func callViewWithAugmentedContext(t *testing.T, wg *sync.WaitGroup, m Manager) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, ContextKey("test"), "pineapple")
	v, err := m.InitiateView(&DummyViewContextCheck{}, ctx)
	wg.Done()
	assert.NoError(t, err)
	assert.Equal(t, "pineapple", v)
}

func newView(t *testing.T, wg *sync.WaitGroup, m Manager) {
	_, err := m.NewView(utils.GenerateUUID(), nil)
	wg.Done()
	assert.Error(t, err)
}

func initiateView(t *testing.T, wg *sync.WaitGroup, m Manager) {
	_, err := m.Initiate(utils.GenerateUUID(), context.Background())
	wg.Done()
	assert.Error(t, err)
}

func getContext(t *testing.T, wg *sync.WaitGroup, m Manager) {
	_, err := m.Context("a context")
	wg.Done()
	assert.Error(t, err)
}

func start(t *testing.T, wg *sync.WaitGroup, m Manager, ctx context.Context) {
	m.Start(ctx)
	wg.Done()
}
