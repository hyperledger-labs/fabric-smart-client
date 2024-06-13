/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package manager_test

import (
	"sync"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/manager"
	mock2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/manager/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver/mock"
	registry2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/registry"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
)

type Manager interface {
	InitiateView(f view.View) (interface{}, error)
	Context(id string) (view.Context, error)
	RegisterFactory(id string, factory driver.Factory) error
	NewView(id string, in []byte) (f view.View, err error)
	Initiate(id string) (interface{}, error)
	RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy interface{}) error
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

type DummyFactory struct{}

func (d *DummyFactory) NewView(in []byte) (view.View, error) {
	time.Sleep(2 * time.Second)
	return nil, nil
}

func TestGetIdentifier(t *testing.T) {
	registry := registry2.New()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))
	manager := manager.New(registry, &mock2.CommLayer{}, &mock.EndpointService{}, idProvider)

	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/manager_test/DummyView", manager.GetIdentifier(DummyView{}))
	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/manager_test/DummyView", manager.GetIdentifier(&DummyView{}))
	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/manager_test/DummyView", manager.GetIdentifier(new(DummyView)))
	assert.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/manager_test/DummyView", manager.GetIdentifier(*(new(DummyView))))
}

func TestManagerRace(t *testing.T) {
	registry := registry2.New()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))
	assert.NoError(t, registry.RegisterService(idProvider))
	assert.NoError(t, registry.RegisterService(&mock2.CommLayer{}))
	assert.NoError(t, registry.RegisterService(&mock.EndpointService{}))
	assert.NoError(t, registry.RegisterService(&mock2.SessionFactory{}))
	manager := manager.New(registry, &mock2.CommLayer{}, &mock.EndpointService{}, idProvider)

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
	registry := registry2.New()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))

	manager := manager.New(registry, &mock2.CommLayer{}, &mock.EndpointService{}, idProvider)
	err := manager.RegisterResponder(&ResponderView{}, &InitiatorView{})
	assert.NoError(t, err)
	responder, _, err := manager.ExistResponderForCaller(manager.GetIdentifier(&InitiatorView{}))
	assert.NoError(t, err)
	res, err := responder.Call(nil)
	assert.NoError(t, err)
	assert.Equal(t, "pineapple", res)

}

func TestRegisterResponderWithViewIdentifier(t *testing.T) {
	registry := registry2.New()
	idProvider := &mock.IdentityProvider{}
	idProvider.DefaultIdentityReturns([]byte("alice"))

	manager := manager.New(registry, &mock2.CommLayer{}, &mock.EndpointService{}, idProvider)
	err := manager.RegisterResponder(&ResponderView{}, manager.GetIdentifier(&InitiatorView{}))
	assert.NoError(t, err)
	responder, _, err := manager.ExistResponderForCaller(manager.GetIdentifier(&InitiatorView{}))
	assert.NoError(t, err)
	res, err := responder.Call(nil)
	assert.NoError(t, err)
	assert.Equal(t, "pineapple", res)
}

func registerFactory(t *testing.T, wg *sync.WaitGroup, m Manager) {
	err := m.RegisterFactory(manager.GenerateUUID(), &DummyFactory{})
	wg.Done()
	assert.NoError(t, err)
}

func registerResponder(t *testing.T, wg *sync.WaitGroup, m Manager) {
	assert.NoError(t, m.RegisterResponderWithIdentity(&DummyView{}, []byte("alice"), &DummyView{}))
	wg.Done()
}

func callView(t *testing.T, wg *sync.WaitGroup, m Manager) {
	_, err := m.InitiateView(&DummyView{})
	wg.Done()
	assert.NoError(t, err)
}

func newView(t *testing.T, wg *sync.WaitGroup, m Manager) {
	_, err := m.NewView(manager.GenerateUUID(), nil)
	wg.Done()
	assert.Error(t, err)
}

func initiateView(t *testing.T, wg *sync.WaitGroup, m Manager) {
	_, err := m.Initiate(manager.GenerateUUID())
	wg.Done()
	assert.Error(t, err)
}

func getContext(t *testing.T, wg *sync.WaitGroup, m Manager) {
	_, err := m.Context("a context")
	wg.Done()
	assert.Error(t, err)
}
