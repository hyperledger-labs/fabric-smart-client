/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
)

func TestRegistry(t *testing.T) {
	registry := view.NewRegistry()
	assert.NotNil(t, registry)

	// RegisterFactory / NewView
	factory := &mock.Factory{}
	err := registry.RegisterFactory("v1", factory)
	assert.NoError(t, err)

	v := &mock.View{}
	factory.NewViewReturns(v, nil)
	v2, err := registry.NewView("v1", nil)
	assert.NoError(t, err)
	assert.Equal(t, v, v2)

	// NewView - factory not found
	_, err = registry.NewView("v2", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no factory found")

	// NewView - panic in factory
	factory.NewViewStub = func(in []byte) (view2.View, error) {
		panic("factory-panic")
	}
	_, err = registry.NewView("v1", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "panic creating view")

	// RegisterResponderFactory
	rfactory := &mock.Factory{}
	responder := &mock.View{}
	rfactory.NewViewReturns(responder, nil)
	err = registry.RegisterResponderFactory(rfactory, "initiator")
	assert.NoError(t, err)

	r, err := registry.GetResponder("initiator")
	assert.NoError(t, err)
	assert.Equal(t, responder, r)

	// GetResponder - not found
	_, err = registry.GetResponder("not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "responder not found")

	// GetResponder - error initiatedBy type
	_, err = registry.GetResponder(123)
	assert.Error(t, err)

	// RegisterResponderWithIdentity / ExistResponderForCaller
	err = registry.RegisterResponderWithIdentity(responder, view2.Identity("id"), "initiator2")
	assert.NoError(t, err)

	r2, id, err := registry.ExistResponderForCaller("initiator2")
	assert.NoError(t, err)
	assert.Equal(t, responder, r2)
	assert.Equal(t, view2.Identity("id"), id)

	// GetView
	err = registry.RegisterResponder(responder, "") // This registers it as initiator
	assert.NoError(t, err)
	
	v3, err := registry.GetView(view.GetIdentifier(responder))
	assert.NoError(t, err)
	assert.Equal(t, responder, v3)

	// GetIdentifier / GetName
	assert.NotEmpty(t, view.GetIdentifier(v))
	assert.NotEmpty(t, view.GetName(v))
	assert.Equal(t, "<nil view>", view.GetIdentifier(nil))
	assert.Equal(t, "<nil view>", view.GetName(nil))

	// GetRegistry
	sp := view.NewServiceProvider()
	sp.RegisterService(registry)
	assert.Equal(t, registry, view.GetRegistry(sp))
}
