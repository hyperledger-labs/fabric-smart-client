/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"reflect"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/stretchr/testify/assert"
)

type MyInterface interface {
	Foo()
}

type MyImpl struct{}

func (m *MyImpl) Foo() {}

func TestSP(t *testing.T) {
	sp := view.NewServiceProvider()
	
	impl := &MyImpl{}
	err := sp.RegisterService(impl)
	assert.NoError(t, err)
	
	// Implementation check
	typ := reflect.TypeOf((*MyInterface)(nil)).Elem()
	assert.True(t, reflect.TypeOf(impl).Implements(typ))

	// Get by interface type
	s2, err := sp.GetService(typ)
	assert.NoError(t, err, "Failed to get by interface type")
	assert.Equal(t, impl, s2)

	// Get by ptr to interface
	ptrToInterface := reflect.TypeOf((*MyInterface)(nil))
	s, err := sp.GetService(ptrToInterface)
	assert.NoError(t, err, "Failed to get by ptr to interface")
	assert.Equal(t, impl, s)
}
