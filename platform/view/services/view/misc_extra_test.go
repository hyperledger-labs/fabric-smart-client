/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	"github.com/stretchr/testify/require"
)

func TestSessionsDelete(t *testing.T) {
	// Already tested in sessions_test.go
}

func TestServiceProviderString(t *testing.T) {
	sp := view.NewServiceProvider()
	err := sp.RegisterService("foo")
	require.NoError(t, err)
	s := sp.String()
	require.Contains(t, s, "services [")
	require.Contains(t, s, "string")
}

func TestStream(t *testing.T) {
	sp := view.NewServiceProvider()
	mockStream := &mock.Stream{}
	err := sp.RegisterService(mockStream)
	require.NoError(t, err)

	s := view.GetStream(sp)
	require.Equal(t, mockStream, s)

	s2, err := view.GetStreamIfExists(sp)
	require.NoError(t, err)
	require.Equal(t, mockStream, s2)

	spEmpty := view.NewServiceProvider()
	require.Panics(t, func() { view.GetStream(spEmpty) })
	_, err = view.GetStreamIfExists(spEmpty)
	require.Error(t, err)
}

func TestWrappedContext(t *testing.T) {
	type contextKey string
	parent := &mock.ParentContext{}
	ctx := context.WithValue(context.Background(), contextKey("key"), "value")
	wrapped := view.WrapContext(parent, ctx)
	require.Equal(t, ctx, wrapped.Context())
}
