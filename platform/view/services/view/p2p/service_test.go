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
	mock2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/p2p/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/require"
)

type viewManagerMock struct {
	HandleResponderCalled chan struct{}

	ExistResponderForCallerFunc func(caller string) (view.View, view.Identity, error)
	GetIdentityFunc             func(endpoint string, pkID []byte) (view.Identity, error)
	NewSessionContextFunc       func(ctx context.Context, contextID string, session view.Session, party view.Identity) (view.Context, bool, error)
	DeleteContextFunc           func(contextID string)
	DefaultIdentityFunc         func() view.Identity
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

func (m *viewManagerMock) NewResponderContext(ctx context.Context, contextID string, session view.Session, me, remote view.Identity) (view.Context, bool, error) {
	if m.NewSessionContextFunc != nil {
		return m.NewSessionContextFunc(ctx, contextID, session, me)
	}
	return &mock.Context{}, true, nil
}

func (m *viewManagerMock) DeleteContext(contextID string) {
	if m.DeleteContextFunc != nil {
		m.DeleteContextFunc(contextID)
	}
}

func (m *viewManagerMock) DefaultIdentity() view.Identity {
	if m.DefaultIdentityFunc != nil {
		return m.DefaultIdentityFunc()
	}
	return view.Identity("me")
}

func TestService(t *testing.T) {
	t.Parallel()
	vm := &viewManagerMock{HandleResponderCalled: make(chan struct{}, 10)}
	cl := &mock2.CommLayer{}
	sess := &mock.Session{}
	ch := make(chan *view.Message, 10)
	cl.MasterSessionReturns(sess, nil)
	sess.ReceiveReturns(ch)

	service := p2p.NewService(vm, vm, cl, vm, p2p.NewDefaultRunner())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := service.Start(ctx)
	require.NoError(t, err)

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
		require.Equal(t, "caller1", caller)
		return &mock.View{}, nil, nil
	}
	vm.NewSessionContextFunc = func(ctx context.Context, contextID string, session view.Session, party view.Identity) (view.Context, bool, error) {
		require.Equal(t, "ctx1", contextID)
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
	t.Parallel()
	vm := &viewManagerMock{}
	cl := &mock2.CommLayer{}
	cl.MasterSessionReturns(nil, errors.New("master session error"))

	service := p2p.NewService(vm, vm, cl, vm, p2p.NewDefaultRunner())
	err := service.Start(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed getting master session")
}

func TestService_HandleResponderError(t *testing.T) {
	t.Parallel()
	vm := &viewManagerMock{
		HandleResponderCalled: make(chan struct{}, 10),
	}
	vm.NewSessionContextFunc = func(ctx context.Context, contextID string, session view.Session, party view.Identity) (view.Context, bool, error) {
		vm.HandleResponderCalled <- struct{}{}
		return &mock.Context{}, true, nil
	}

	cl := &mock2.CommLayer{}
	sess := &mock.Session{}
	ch := make(chan *view.Message, 10)
	cl.MasterSessionReturns(sess, nil)
	sess.ReceiveReturns(ch)

	service := p2p.NewService(vm, vm, cl, vm, p2p.NewDefaultRunner())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := service.Start(ctx)
	require.NoError(t, err)

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
