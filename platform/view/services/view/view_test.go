/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

type testKey string

func TestRunViewNow(t *testing.T) {
	v := &mock.View{}
	v.CallStub = func(ctx view2.Context) (interface{}, error) {
		value, ok := ctx.Context().Value(v).(testKey)
		if !ok {
			panic("value is not a string")
		}
		return value, nil
	}

	ctx := t.Context()
	ctx = context.WithValue(ctx, v, testKey("test"))

	parent := &mock.ParentContext{}
	parent.ContextReturns(t.Context())
	parent.StartSpanFromReturns(ctx, &noop.Span{})
	value, err := view.RunViewNow(parent, v, view2.WithContext(ctx))
	require.NoError(t, err)
	assert.Equal(t, testKey("test"), value)
}

func TestRunViewNow_CallOption(t *testing.T) {
	parent := &mock.ParentContext{}
	ctx := t.Context()
	ctx = context.WithValue(ctx, testKey("k"), "val")
	parent.ContextReturns(t.Context())
	parent.StartSpanFromReturns(ctx, &noop.Span{})

	call := func(c view2.Context) (interface{}, error) {
		return c.Context().Value(testKey("k")), nil
	}

	res, err := view.RunViewNow(parent, nil, view2.WithViewCall(call), view2.WithContext(ctx))
	require.NoError(t, err)
	assert.Equal(t, "val", res)
}

func TestRunViewNow_AsInitiator_NoSession(t *testing.T) {
	v := &mock.View{}
	parent := &mock.ParentContext{}
	parent.ContextReturns(t.Context())
	parent.StartSpanFromReturns(t.Context(), &noop.Span{})
	// ensure Session() returns nil to trigger the error path
	parent.SessionReturns(nil)

	_, err := view.RunViewNow(parent, v, view2.AsInitiator())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot convert a non-responder context to an initiator context")
}

func TestRunViewNow_AsInitiator_PutSessionError(t *testing.T) {
	v := &mock.View{}
	parent := &mock.ParentContext{}
	parent.ContextReturns(t.Context())
	parent.StartSpanFromReturns(t.Context(), &noop.Span{})

	// create a mock session with Info returning a caller
	s := &mock.Session{}
	s.InfoReturns(view2.SessionInfo{Caller: view2.Identity("caller1")})
	parent.SessionReturns(s)
	parent.PutSessionReturns(errors.New("put failed"))

	_, err := view.RunViewNow(parent, v, view2.AsInitiator())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed registering default session as initiated by")
}

func TestRunViewNow_PanicInView_CallsCleanupAndReturnsError(t *testing.T) {
	v := &mock.View{}
	v.CallStub = func(ctx view2.Context) (interface{}, error) {
		panic(errors.New("boom"))
	}

	parent := &mock.ParentContext{}
	parent.ContextReturns(t.Context())
	parent.StartSpanFromReturns(t.Context(), &noop.Span{})

	called := false
	parent.CleanupCalls(func() { called = true })

	// use SameContext so cc becomes WrapContext(ctx, newCtx) and Cleanup() calls parent.Cleanup
	_, err := view.RunViewNow(parent, v, view2.WithSameContext())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "caught panic")
	assert.True(t, called, "expected parent.Cleanup to be invoked")
}

func TestRunViewNow_NoViewAndNoCall(t *testing.T) {
	parent := &mock.ParentContext{}
	parent.ContextReturns(t.Context())
	parent.StartSpanFromReturns(t.Context(), &noop.Span{})

	_, err := view.RunViewNow(parent, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no view passed")
}
