/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
)

func TestRunViewNow(t *testing.T) {
	parent := &mock.ParentContext{}
	parent.ContextReturns(context.Background())
	parent.StartSpanFromStub = func(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
		return ctx, trace.SpanFromContext(ctx)
	}

	v := &mock.View{}
	v.CallReturns("result", nil)

	res, err := view.RunViewNow(parent, v)
	assert.NoError(t, err)
	assert.Equal(t, "result", res)
}

func TestRunViewNow_CallOption(t *testing.T) {
	parent := &mock.ParentContext{}
	parent.ContextReturns(context.Background())
	parent.StartSpanFromStub = func(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
		return ctx, trace.SpanFromContext(ctx)
	}

	call := func(viewCtx view2.Context) (any, error) {
		return "call-result", nil
	}

	res, err := view.RunViewNow(parent, nil, view2.WithViewCall(call))
	assert.NoError(t, err)
	assert.Equal(t, "call-result", res)
}

func TestRunViewNow_AsInitiator_NoSession(t *testing.T) {
	parent := &mock.ParentContext{}
	parent.ContextReturns(context.Background())
	parent.StartSpanFromStub = func(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
		return ctx, trace.SpanFromContext(ctx)
	}
	parent.SessionReturns(nil)

	v := &mock.View{}

	_, err := view.RunViewNow(parent, v, view2.AsInitiator())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot convert a non-responder context to an initiator context")
}

func TestRunViewNow_AsInitiator_PutSessionError(t *testing.T) {
	parent := &mock.ParentContext{}
	parent.ContextReturns(context.Background())
	parent.StartSpanFromStub = func(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
		return ctx, trace.SpanFromContext(ctx)
	}
	session := &mock.Session{}
	session.InfoReturns(view2.SessionInfo{Caller: view2.Identity("alice")})
	parent.SessionReturns(session)

	v := &mock.View{}
	parent.PutSessionReturns(errors.New("put-error"))

	_, err := view.RunViewNow(parent, v, view2.AsInitiator())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed registering default session")
}

func TestRunViewNow_PanicInView_CallsCleanupAndReturnsError(t *testing.T) {
	parent := &mock.ParentContext{}
	parent.ContextReturns(context.Background())
	parent.StartSpanFromStub = func(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
		return ctx, trace.SpanFromContext(ctx)
	}

	v := &mock.View{}
	v.CallStub = func(viewCtx view2.Context) (any, error) {
		panic("boom")
	}

	res, err := view.RunViewNow(parent, v)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "caught panic: boom")
}

func TestRunViewNow_NoViewAndNoCall(t *testing.T) {
	parent := &mock.ParentContext{}
	parent.ContextReturns(context.Background())
	parent.StartSpanFromStub = func(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
		return ctx, trace.SpanFromContext(ctx)
	}

	_, err := view.RunViewNow(parent, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no view passed")
}

func TestRunCall(t *testing.T) {
	ctx := &mock.Context{}
	call := func(viewCtx view2.Context) (any, error) {
		return "res", nil
	}
	ctx.RunViewReturns("res", nil)

	res, err := view.RunCall(ctx, call)
	assert.NoError(t, err)
	assert.Equal(t, "res", res)
}

func TestAsResponder(t *testing.T) {
	ctx := &mock.Context{}
	session := &mock.Session{}
	call := func(viewCtx view2.Context) (any, error) {
		return "res", nil
	}
	ctx.RunViewReturns("res", nil)

	res, err := view.AsResponder(ctx, session, call)
	assert.NoError(t, err)
	assert.Equal(t, "res", res)
}

func TestAsInitiatorCall(t *testing.T) {
	ctx := &mock.Context{}
	v := &mock.View{}
	call := func(viewCtx view2.Context) (any, error) {
		return "res", nil
	}
	ctx.RunViewReturns("res", nil)

	res, err := view.AsInitiatorCall(ctx, v, call)
	assert.NoError(t, err)
	assert.Equal(t, "res", res)
}

func TestAsInitiatorView(t *testing.T) {
	ctx := &mock.Context{}
	v := &mock.View{}
	ctx.RunViewReturns("res", nil)

	res, err := view.AsInitiatorView(ctx, v)
	assert.NoError(t, err)
	assert.Equal(t, "res", res)
}

func TestRunView(t *testing.T) {
	ctx := &mock.Context{}
	v := &mock.View{}

	view.RunView(ctx, v)
	// it's a goroutine, hard to test easily but we call it for coverage
}
