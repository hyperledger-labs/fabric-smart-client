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
	"github.com/stretchr/testify/assert"
)

func TestSessionsDelete(t *testing.T) {
	// Already tested in sessions_test.go
}

func TestServiceProviderString(t *testing.T) {
	sp := view.NewServiceProvider()
	err := sp.RegisterService("foo")
	assert.NoError(t, err)
	s := sp.String()
	assert.Contains(t, s, "services [")
	assert.Contains(t, s, "string")
}

func TestStream(t *testing.T) {
	sp := view.NewServiceProvider()
	mockStream := &mock.Stream{}
	err := sp.RegisterService(mockStream)
	assert.NoError(t, err)

	s := view.GetStream(sp)
	assert.Equal(t, mockStream, s)

	s2, err := view.GetStreamIfExists(sp)
	assert.NoError(t, err)
	assert.Equal(t, mockStream, s2)

	spEmpty := view.NewServiceProvider()
	assert.Panics(t, func() { view.GetStream(spEmpty) })
	_, err = view.GetStreamIfExists(spEmpty)
	assert.Error(t, err)
}

func TestWrappedContext(t *testing.T) {
	type contextKey string
	parent := &mock.ParentContext{}
	ctx := context.WithValue(context.Background(), contextKey("key"), "value")
	wrapped := view.WrapContext(parent, ctx)
	assert.Equal(t, ctx, wrapped.Context())
}
