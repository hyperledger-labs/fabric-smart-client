/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

type MyInterface interface {
	Foo()
}

type MyImpl struct{}

func (m *MyImpl) Foo() {}

func TestSP(t *testing.T) {
	t.Parallel()
	sp := view.NewServiceProvider()

	impl := &MyImpl{}
	err := sp.RegisterService(impl)
	require.NoError(t, err)

	// Implementation check
	typ := reflect.TypeFor[MyInterface]()
	require.True(t, reflect.TypeFor[*MyImpl]().Implements(typ))

	// Get by interface type
	s2, err := sp.GetService(typ)
	require.NoError(t, err, "Failed to get by interface type")
	require.Equal(t, impl, s2)

	// Get by ptr to interface
	ptrToInterface := reflect.TypeFor[*MyInterface]()
	s, err := sp.GetService(ptrToInterface)
	require.NoError(t, err, "Failed to get by ptr to interface")
	require.Equal(t, impl, s)
}
