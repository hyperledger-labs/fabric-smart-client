/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package p2p_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/p2p"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
)

//go:generate counterfeiter -o mock/view_manager.go -fake-name ViewManager . ViewManager
type ViewManager interface {
	ExistResponderForCaller(caller string) (view.View, view.Identity, error)
	NewSessionContext(ctx context.Context, contextID string, session view.Session, party view.Identity) (view.Context, bool, error)
	DeleteContext(id view.Identity, contextID string)
	Me() view.Identity
	SetContext(ctx context.Context)
}

type EndpointService interface {
	GetIdentity(endpoint string, pkID []byte) (view.Identity, error)
}

type viewManagerMock struct {
	HandleResponderCalled chan struct{}

	ExistResponderForCallerFunc func(caller string) (view.View, view.Identity, error)
	GetIdentityFunc             func(endpoint string, pkID []byte) (view.Identity, error)
	NewSessionContextFunc       func(ctx context.Context, contextID string, session view.Session, party view.Identity) (view.Context, bool, error)
	DeleteContextFunc           func(id view.Identity, contextID string)
	MeFunc                      func() view.Identity
	SetContextFunc              func(ctx context.Context)
}

func (m *viewManagerMock) ExistResponderForCaller(caller string) (view.View, view.Identity, error) {
	if m.ExistResponderForCallerFunc != nil {
		return m.ExistResponderForCallerFunc(caller)
	}
	return &mock.View{}, nil, nil
}

func (m *viewManagerMock) GetIdentity(endpoint string, pkID []byte) (view.Identity, error) {
	if m.GetIdentityFunc != nil {
		return m.GetIdentityFunc(endpoint, pkID)
	}
	return view.Identity("caller"), nil
}

func (m *viewManagerMock) NewSessionContext(ctx context.Context, contextID string, session view.Session, party view.Identity) (view.Context, bool, error) {
	if m.NewSessionContextFunc != nil {
		return m.NewSessionContextFunc(ctx, contextID, session, party)
	}
	return &mock.Context{}, true, nil
}

func (m *viewManagerMock) DeleteContext(id view.Identity, contextID string) {
	if m.DeleteContextFunc != nil {
		m.DeleteContextFunc(id, contextID)
	}
}

func (m *viewManagerMock) Me() view.Identity {
	if m.MeFunc != nil {
		return m.MeFunc()
	}
	return view.Identity("me")
}

func (m *viewManagerMock) SetContext(ctx context.Context) {
	if m.SetContextFunc != nil {
		m.SetContextFunc(ctx)
	}
}

func TestService(t *testing.T) {
	vm := &viewManagerMock{HandleResponderCalled: make(chan struct{}, 10)}
	cl := &mock.CommLayer{}
	sess := &mock.Session{}
	ch := make(chan *view.Message, 10)
	cl.MasterSessionReturns(sess, nil)
	sess.ReceiveReturns(ch)

	service := p2p.NewService(vm, cl, vm, p2p.NewDefaultRunner())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := service.Start(ctx)
	assert.NoError(t, err)

	// Send a message
	msg := &view.Message{
		ContextID:    "ctx1",
		SessionID:    "sess1",
		Caller:       "caller1",
		FromEndpoint: "endpoint1",
		FromPKID:     []byte("pkid1"),
		Ctx:          ctx,
	}

	vm.ExistResponderForCallerFunc = func(caller string) (view.View, view.Identity, error) {
		assert.Equal(t, "caller1", caller)
		return &mock.View{}, nil, nil
	}
	vm.NewSessionContextFunc = func(ctx context.Context, contextID string, session view.Session, party view.Identity) (view.Context, bool, error) {
		assert.Equal(t, "ctx1", contextID)
		vm.HandleResponderCalled <- struct{}{}
		return &mock.Context{}, true, nil
	}

	ch <- msg

	select {
	case <-vm.HandleResponderCalled:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("NewSessionContext was not called")
	}
}

func TestService_MasterSessionError(t *testing.T) {
	vm := &viewManagerMock{}
	cl := &mock.CommLayer{}
	cl.MasterSessionReturns(nil, errors.New("master session error"))

	service := p2p.NewService(vm, cl, vm, p2p.NewDefaultRunner())
	err := service.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed getting master session")
}

func TestService_HandleResponderError(t *testing.T) {
	vm := &viewManagerMock{
		HandleResponderCalled: make(chan struct{}, 10),
	}
	vm.NewSessionContextFunc = func(ctx context.Context, contextID string, session view.Session, party view.Identity) (view.Context, bool, error) {
		vm.HandleResponderCalled <- struct{}{}
		return &mock.Context{}, true, nil
	}

	cl := &mock.CommLayer{}
	sess := &mock.Session{}
	ch := make(chan *view.Message, 10)
	cl.MasterSessionReturns(sess, nil)
	sess.ReceiveReturns(ch)

	service := p2p.NewService(vm, cl, vm, p2p.NewDefaultRunner())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := service.Start(ctx)
	assert.NoError(t, err)

	// Send a message
	msg := &view.Message{
		ContextID: "ctx1",
		Ctx:       ctx,
	}
	ch <- msg

	select {
	case <-vm.HandleResponderCalled:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("NewSessionContext was not called")
	}
}
