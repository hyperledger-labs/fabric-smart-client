/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestRegistry(t *testing.T) {
	t.Parallel()
	registry := view.NewRegistry()
	require.NotNil(t, registry)

	// RegisterFactory / NewView
	factory := &mock.Factory{}
	err := registry.RegisterFactory("v1", factory)
	require.NoError(t, err)

	v := &mock.View{}
	factory.NewViewReturns(v, nil)
	v2, err := registry.NewView("v1", nil)
	require.NoError(t, err)
	require.Equal(t, v, v2)

	// NewView - factory not found
	_, err = registry.NewView("v2", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no factory found")

	// NewView - panic in factory
	factory.NewViewStub = func(in []byte) (view2.View, error) {
		panic("factory-panic")
	}
	_, err = registry.NewView("v1", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "panic creating view")

	// RegisterResponderFactory
	rfactory := &mock.Factory{}
	responder := &mock.View{}
	rfactory.NewViewReturns(responder, nil)
	err = registry.RegisterResponderFactory(rfactory, "initiator")
	require.NoError(t, err)

	r, err := registry.GetResponder("initiator")
	require.NoError(t, err)
	require.Equal(t, responder, r)

	// GetResponder - not found
	_, err = registry.GetResponder("not-found")
	require.Error(t, err)
	require.Contains(t, err.Error(), "responder not found")

	// GetResponder - error initiatedBy type
	_, err = registry.GetResponder(123)
	require.Error(t, err)

	// RegisterResponderWithIdentity / ExistResponderForCaller
	err = registry.RegisterResponderWithIdentity(responder, view2.Identity("id"), "initiator2")
	require.NoError(t, err)

	// Test RegisterResponderWithIdentity with view
	err = registry.RegisterResponderWithIdentity(responder, view2.Identity("id"), v)
	require.NoError(t, err)

	// Test RegisterResponderWithIdentity with invalid type
	err = registry.RegisterResponderWithIdentity(responder, view2.Identity("id"), 123)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be a view or a string")

	// Test RegisterResponderFactory error path
	factory.NewViewReturns(nil, errors.New("factory-error"))
	err = registry.RegisterResponderFactory(factory, "initiator3")
	require.Error(t, err)

	r2, id, err := registry.ExistResponderForCaller("initiator2")
	require.NoError(t, err)
	require.Equal(t, responder, r2)
	require.Equal(t, view2.Identity("id"), id)

	// ExistResponderForCaller - not found
	_, _, err = registry.ExistResponderForCaller("not-found")
	require.Error(t, err)

	// GetView - not found
	_, err = registry.GetView("not-found")
	require.Error(t, err)

	// GetView
	err = registry.RegisterResponder(responder, "") // This registers it as initiator
	require.NoError(t, err)

	v3, err := registry.GetView(view.GetIdentifier(responder))
	require.NoError(t, err)
	require.Equal(t, responder, v3)

	// GetIdentifier / GetName
	require.NotEmpty(t, view.GetIdentifier(v))
	require.NotEmpty(t, registry.GetIdentifier(v))
	require.NotEmpty(t, view.GetName(v))
	require.Equal(t, "<nil view>", view.GetIdentifier(nil))
	require.Equal(t, "<nil view>", view.GetName(nil))

	// GetRegistry
	sp := view.NewServiceProvider()
	err = sp.RegisterService(registry)
	require.NoError(t, err)
	require.Equal(t, registry, view.GetRegistry(sp))
}
