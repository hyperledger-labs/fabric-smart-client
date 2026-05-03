/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestRunViewNow(t *testing.T) {
	t.Parallel()
	parent := &mock.ParentContext{}
	parent.ContextReturns(context.Background())
	parent.StartSpanFromStub = func(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
		return ctx, trace.SpanFromContext(ctx)
	}

	v := &mock.View{}
	v.CallReturns("result", nil)

	res, err := view.RunViewNow(parent, v)
	require.NoError(t, err)
	require.Equal(t, "result", res)
}

func TestRunViewNow_CallOption(t *testing.T) {
	t.Parallel()
	parent := &mock.ParentContext{}
	parent.ContextReturns(context.Background())
	parent.StartSpanFromStub = func(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
		return ctx, trace.SpanFromContext(ctx)
	}

	call := func(viewCtx view2.Context) (any, error) {
		return "call-result", nil
	}

	res, err := view.RunViewNow(parent, nil, view2.WithViewCall(call))
	require.NoError(t, err)
	require.Equal(t, "call-result", res)
}

func TestRunViewNow_AsInitiator_NoSession(t *testing.T) {
	t.Parallel()
	parent := &mock.ParentContext{}
	parent.ContextReturns(context.Background())
	parent.StartSpanFromStub = func(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
		return ctx, trace.SpanFromContext(ctx)
	}
	parent.SessionReturns(nil)

	v := &mock.View{}

	_, err := view.RunViewNow(parent, v, view2.AsInitiator())
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot convert a non-responder context to an initiator context")
}

func TestRunViewNow_AsInitiator_PutSessionError(t *testing.T) {
	t.Parallel()
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
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed registering default session")
}

func TestRunViewNow_PanicInView_CallsCleanupAndReturnsError(t *testing.T) {
	t.Parallel()
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
	require.Error(t, err)
	require.Nil(t, res)
	require.Contains(t, err.Error(), "caught panic: boom")
}

func TestRunViewNow_NoViewAndNoCall(t *testing.T) {
	t.Parallel()
	parent := &mock.ParentContext{}
	parent.ContextReturns(context.Background())
	parent.StartSpanFromStub = func(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
		return ctx, trace.SpanFromContext(ctx)
	}

	_, err := view.RunViewNow(parent, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no view passed")
}

func TestRunCall(t *testing.T) {
	t.Parallel()
	ctx := &mock.Context{}
	call := func(viewCtx view2.Context) (any, error) {
		return "res", nil
	}
	ctx.RunViewReturns("res", nil)

	res, err := view.RunCall(ctx, call)
	require.NoError(t, err)
	require.Equal(t, "res", res)
}

func TestAsResponder(t *testing.T) {
	t.Parallel()
	ctx := &mock.Context{}
	session := &mock.Session{}
	call := func(viewCtx view2.Context) (any, error) {
		return "res", nil
	}
	ctx.RunViewReturns("res", nil)

	res, err := view.AsResponder(ctx, session, call)
	require.NoError(t, err)
	require.Equal(t, "res", res)
}

func TestAsInitiatorCall(t *testing.T) {
	t.Parallel()
	ctx := &mock.Context{}
	v := &mock.View{}
	call := func(viewCtx view2.Context) (any, error) {
		return "res", nil
	}
	ctx.RunViewReturns("res", nil)

	res, err := view.AsInitiatorCall(ctx, v, call)
	require.NoError(t, err)
	require.Equal(t, "res", res)
}

func TestAsInitiatorView(t *testing.T) {
	t.Parallel()
	ctx := &mock.Context{}
	v := &mock.View{}
	ctx.RunViewReturns("res", nil)

	res, err := view.AsInitiatorView(ctx, v)
	require.NoError(t, err)
	require.Equal(t, "res", res)
}

func TestRunView(t *testing.T) {
	t.Parallel()
	ctx := &mock.Context{}
	v := &mock.View{}

	view.RunView(ctx, v)
	// it's a goroutine, hard to test easily but we call it for coverage
}

func newRecordingParent(t *testing.T) (*mock.ParentContext, *tracetest.InMemoryExporter) {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	tracer := tp.Tracer("test")

	parent := &mock.ParentContext{}
	parent.ContextReturns(context.Background())
	parent.StartSpanFromStub = func(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
		return tracer.Start(ctx, name, opts...)
	}
	return parent, exp
}

func spanSuccessAttr(spans tracetest.SpanStubs) (bool, bool) {
	for _, s := range spans {
		for _, a := range s.Attributes {
			if a.Key == attribute.Key(view.SuccessLabel) {
				return a.Value.AsBool(), true
			}
		}
	}
	return false, false
}

func TestRunViewNow_SuccessLabel_TrueOnSuccess(t *testing.T) {
	t.Parallel()
	parent, exp := newRecordingParent(t)

	v := &mock.View{}
	v.CallReturns("result", nil)

	_, err := view.RunViewNow(parent, v)
	require.NoError(t, err)

	val, found := spanSuccessAttr(exp.GetSpans())
	require.True(t, found, "success attribute not set on span")
	require.True(t, val, "expected success=true for a successful view execution")
}

func TestRunViewNow_SuccessLabel_FalseOnError(t *testing.T) {
	t.Parallel()
	parent, exp := newRecordingParent(t)

	v := &mock.View{}
	v.CallReturns(nil, errors.New("view-error"))

	_, err := view.RunViewNow(parent, v)
	require.Error(t, err)

	val, found := spanSuccessAttr(exp.GetSpans())
	require.True(t, found, "success attribute not set on span")
	require.False(t, val, "expected success=false for a failed view execution")
}
