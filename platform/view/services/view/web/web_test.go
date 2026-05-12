/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	server2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestDispatcher(t *testing.T) {
	t.Parallel()
	h := server2.NewHttpHandler()
	d := newDispatcher(h)

	// vc is nil
	req, _ := http.NewRequest("PUT", "/v1/Views/fid", nil)
	reqctx := &server2.ReqContext{
		Req:   req,
		Vars:  map[string]string{"View": "fid"},
		Query: []byte("input"),
	}
	resp, code := d.HandleRequest(reqctx)
	require.Equal(t, 500, code)
	require.Equal(t, "internal error", resp.(*server2.ResponseErr).Reason)

	// success
	vc := &fakeViewCaller{}
	d.WireViewCaller(vc)
	resp, code = d.HandleRequest(reqctx)
	require.Equal(t, 200, code)
	require.Equal(t, "result", resp)

	// error
	vc.err = fmt.Errorf("caller error")
	resp, code = d.HandleRequest(reqctx)
	require.Equal(t, 500, code)
	require.Contains(t, resp.(*server2.ResponseErr).Reason, "caller error")

	// ParsePayload
	p, err := d.ParsePayload([]byte("data"))
	require.NoError(t, err)
	require.Equal(t, []byte("data"), p)

	// WireStreamViewCaller
	d.WireStreamViewCaller(vc)
}

func TestViewHandler(t *testing.T) {
	t.Parallel()
	vm := &fakeViewManager{}
	ip := &fakeIdentityProvider{}
	tp := noop.NewTracerProvider()

	h := server2.NewHttpHandler()
	InstallViewHandler(vm, ip, h, tp)

	c := newViewClient(vm, ip, tp)
	vh := &viewHandler{c: c}

	// CallView success: byte slice
	req, _ := http.NewRequest("PUT", "/v1/Views/fid", nil)
	reqctx := &server2.ReqContext{Req: req}
	resp, err := vh.CallView(reqctx, "fid", []byte("input"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, []byte("result"), resp.(*protos.CommandResponse_CallViewResponse).CallViewResponse.Result)

	// CallView error
	vm.err = fmt.Errorf("manager error")
	_, err = vh.CallView(reqctx, "fid", []byte("input"))
	require.Error(t, err)

	// CallView success: non-byte result marshaled to JSON
	vm.err = nil
	vm.initErr = nil
	vm.result = map[string]string{"foo": "bar"}
	resp, err = vh.CallView(reqctx, "fid", []byte("input"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	expectedJSON, _ := json.Marshal(map[string]string{"foo": "bar"})
	require.Equal(t, expectedJSON, resp.(*protos.CommandResponse_CallViewResponse).CallViewResponse.Result)

	// StreamCallView error path without real websocket
	w := httptest.NewRecorder()
	reqctx.ResponseWriter = w
	_, err = vh.StreamCallView(reqctx, "fid", []byte("input"))
	require.Error(t, err)
}

func TestClientCallView(t *testing.T) {
	t.Parallel()
	vm := &fakeViewManager{}
	ip := &fakeIdentityProvider{}
	tp := noop.NewTracerProvider()
	c := newViewClient(vm, ip, tp)

	// CallView success
	res, err := c.CallView("fid", []byte("input"), t.Context())
	require.NoError(t, err)
	require.Equal(t, []byte("result"), res)

	// CallView error: NewView fails
	vm.err = fmt.Errorf("new view error")
	_, err = c.CallView("fid", []byte("input"), t.Context())
	require.Error(t, err)

	// CallView error: InitiateView fails
	vm.err = nil
	vm.initErr = fmt.Errorf("initiate view error")
	_, err = c.CallView("fid", []byte("input"), t.Context())
	require.Error(t, err)
}

func TestClientStreamCallView(t *testing.T) {
	t.Parallel()
	vm := &fakeViewManager{}
	ip := &fakeIdentityProvider{}
	tp := noop.NewTracerProvider()
	c := newViewClient(vm, ip, tp)

	// Setup valid websocket scenario
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = c.StreamCallView("fid", w, r)
	}))
	t.Cleanup(ts.Close)

	wsURL := "ws" + ts.URL[4:]
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })

	// Send input payload conforming to server.Input format
	inputMsg, _ := json.Marshal(server2.Input{Raw: []byte("input")})
	err = ws.WriteMessage(websocket.TextMessage, inputMsg)
	require.NoError(t, err)

	// Read output response conforming to server.Output format
	_, msg, err := ws.ReadMessage()
	require.NoError(t, err)
	var out server2.Output
	err = json.Unmarshal(msg, &out)
	require.NoError(t, err)
	require.Equal(t, []byte("result"), out.Raw)

	// Test stream failures using custom http server handlers
	t.Run("NewView failure", func(t *testing.T) {
		t.Parallel()
		vmErr := &fakeViewManager{err: fmt.Errorf("new view error")}
		cErr := newViewClient(vmErr, ip, tp)
		tsErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := cErr.StreamCallView("fid", w, r)
			require.ErrorContains(t, err, "new view error")
		}))
		t.Cleanup(tsErr.Close)
		wsErr, _, _ := websocket.DefaultDialer.Dial("ws"+tsErr.URL[4:], nil)
		if wsErr != nil {
			t.Cleanup(func() { _ = wsErr.Close() })
			_ = wsErr.WriteMessage(websocket.TextMessage, inputMsg)
		}
	})

	t.Run("InitiateContext failure", func(t *testing.T) {
		t.Parallel()
		vmErr := &fakeViewManager{initCtxErr: fmt.Errorf("init context error")}
		cErr := newViewClient(vmErr, ip, tp)
		tsErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := cErr.StreamCallView("fid", w, r)
			require.ErrorContains(t, err, "init context error")
		}))
		t.Cleanup(tsErr.Close)
		wsErr, _, _ := websocket.DefaultDialer.Dial("ws"+tsErr.URL[4:], nil)
		if wsErr != nil {
			t.Cleanup(func() { _ = wsErr.Close() })
			_ = wsErr.WriteMessage(websocket.TextMessage, inputMsg)
		}
	})

	t.Run("RunView failure", func(t *testing.T) {
		t.Parallel()
		vmErr := &fakeViewManager{ctxRunErr: fmt.Errorf("run view error")}
		cErr := newViewClient(vmErr, ip, tp)
		tsErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := cErr.StreamCallView("fid", w, r)
			require.ErrorContains(t, err, "run view error")
		}))
		t.Cleanup(tsErr.Close)
		wsErr, _, _ := websocket.DefaultDialer.Dial("ws"+tsErr.URL[4:], nil)
		if wsErr != nil {
			t.Cleanup(func() { _ = wsErr.Close() })
			_ = wsErr.WriteMessage(websocket.TextMessage, inputMsg)
		}
	})

	t.Run("PutService failure", func(t *testing.T) {
		t.Parallel()
		vmErr := &fakeViewManager{putServiceErr: fmt.Errorf("put service error")}
		cErr := newViewClient(vmErr, ip, tp)
		tsErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := cErr.StreamCallView("fid", w, r)
			require.ErrorContains(t, err, "registering stream command server")
		}))
		t.Cleanup(tsErr.Close)
		wsErr, _, _ := websocket.DefaultDialer.Dial("ws"+tsErr.URL[4:], nil)
		if wsErr != nil {
			t.Cleanup(func() { _ = wsErr.Close() })
			_ = wsErr.WriteMessage(websocket.TextMessage, inputMsg)
		}
	})

	t.Run("Non-byte RunView result marshaling", func(t *testing.T) {
		t.Parallel()
		vmStruct := &fakeViewManager{ctxResult: map[string]string{"status": "ok"}}
		cStruct := newViewClient(vmStruct, ip, tp)
		tsStruct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = cStruct.StreamCallView("fid", w, r)
		}))
		t.Cleanup(tsStruct.Close)
		wsStruct, _, _ := websocket.DefaultDialer.Dial("ws"+tsStruct.URL[4:], nil)
		require.NotNil(t, wsStruct)
		t.Cleanup(func() { _ = wsStruct.Close() })
		_ = wsStruct.WriteMessage(websocket.TextMessage, inputMsg)
		_, respMsg, err := wsStruct.ReadMessage()
		require.NoError(t, err)
		var outStruct server2.Output
		_ = json.Unmarshal(respMsg, &outStruct)
		expectedJSON, _ := json.Marshal(map[string]string{"status": "ok"})
		require.Equal(t, expectedJSON, outStruct.Raw)
	})

	t.Run("Non-mutable context error", func(t *testing.T) {
		t.Parallel()
		vmNonMutable := &fakeViewManager{nonMutableCtx: true}
		cNonMutable := newViewClient(vmNonMutable, ip, tp)
		tsNonMutable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := cNonMutable.StreamCallView("fid", w, r)
			require.ErrorContains(t, err, "expected a mutable context")
		}))
		t.Cleanup(tsNonMutable.Close)
		wsNM, _, _ := websocket.DefaultDialer.Dial("ws"+tsNonMutable.URL[4:], nil)
		if wsNM != nil {
			t.Cleanup(func() { _ = wsNM.Close() })
			_ = wsNM.WriteMessage(websocket.TextMessage, inputMsg)
		}
	})
}

func TestViewCallFunc(t *testing.T) {
	t.Parallel()
	f := viewCallFunc(func(context *server2.ReqContext, vid string, input []byte) (any, error) {
		return "result", nil
	})
	res, err := f.CallView(nil, "vid", nil)
	require.NoError(t, err)
	require.Equal(t, "result", res)
}

type fakeViewCaller struct {
	err error
}

func (f *fakeViewCaller) CallView(context *server2.ReqContext, vid string, input []byte) (any, error) {
	if f.err != nil {
		return nil, f.err
	}
	return "result", nil
}

type fakeViewManager struct {
	err           error
	initErr       error
	initCtxErr    error
	ctxRunErr     error
	putServiceErr error
	result        any
	ctxResult     any
	nonMutableCtx bool
}

func (f *fakeViewManager) NewView(id string, in []byte) (view2.View, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &fakeView{}, nil
}

func (f *fakeViewManager) InitiateView(ctx context.Context, v view2.View) (any, error) {
	if f.initErr != nil {
		return nil, f.initErr
	}
	if f.result != nil {
		return f.result, nil
	}
	return []byte("result"), nil
}

func (f *fakeViewManager) InitiateContext(ctx context.Context, v view2.View) (view2.Context, error) {
	if f.initCtxErr != nil {
		return nil, f.initCtxErr
	}
	if f.nonMutableCtx {
		return &fakeNonMutableViewContext{}, nil
	}
	res := f.ctxResult
	if res == nil {
		res = []byte("result")
	}
	return &fakeViewContext{runErr: f.ctxRunErr, putServiceErr: f.putServiceErr, result: res}, nil
}

func (f *fakeViewManager) DeleteContext(contextID string) {}

type fakeView struct{}

func (f *fakeView) Call(context view2.Context) (interface{}, error) {
	return nil, nil
}

type fakeIdentityProvider struct{}

func (f *fakeIdentityProvider) DefaultIdentity() view2.Identity { return nil }
func (f *fakeIdentityProvider) Admins() []view2.Identity        { return nil }
func (f *fakeIdentityProvider) Clients() []view2.Identity       { return nil }

type fakeViewContext struct {
	runErr        error
	putServiceErr error
	result        any
}

func (f *fakeViewContext) ID() string { return "ctx-id" }
func (f *fakeViewContext) RunView(v view2.View, opts ...view2.RunViewOption) (any, error) {
	return f.result, f.runErr
}
func (f *fakeViewContext) Context() context.Context      { return context.Background() }
func (f *fakeViewContext) GetService(v any) (any, error) { return nil, nil }
func (f *fakeViewContext) Me() view2.Identity            { return nil }
func (f *fakeViewContext) IsMe(id view2.Identity) bool   { return false }
func (f *fakeViewContext) Initiator() view2.View         { return nil }
func (f *fakeViewContext) GetSession(c view2.View, p view2.Identity, b ...view2.View) (view2.Session, error) {
	return nil, nil
}

func (f *fakeViewContext) GetSessionByID(id string, p view2.Identity) (view2.Session, error) {
	return nil, nil
}
func (f *fakeViewContext) Session() view2.Session  { return nil }
func (f *fakeViewContext) OnError(callback func()) {}
func (f *fakeViewContext) StartSpanFrom(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}
func (f *fakeViewContext) ResetSessions() error { return nil }
func (f *fakeViewContext) PutService(v any) error {
	return f.putServiceErr
}

type fakeNonMutableViewContext struct{}

func (f *fakeNonMutableViewContext) ID() string { return "ctx-id" }
func (f *fakeNonMutableViewContext) RunView(v view2.View, opts ...view2.RunViewOption) (any, error) {
	return []byte("result"), nil
}
func (f *fakeNonMutableViewContext) Context() context.Context      { return context.Background() }
func (f *fakeNonMutableViewContext) GetService(v any) (any, error) { return nil, nil }
func (f *fakeNonMutableViewContext) Me() view2.Identity            { return nil }
func (f *fakeNonMutableViewContext) IsMe(id view2.Identity) bool   { return false }
func (f *fakeNonMutableViewContext) Initiator() view2.View         { return nil }
func (f *fakeNonMutableViewContext) GetSession(c view2.View, p view2.Identity, b ...view2.View) (view2.Session, error) {
	return nil, nil
}

func (f *fakeNonMutableViewContext) GetSessionByID(id string, p view2.Identity) (view2.Session, error) {
	return nil, nil
}
func (f *fakeNonMutableViewContext) Session() view2.Session  { return nil }
func (f *fakeNonMutableViewContext) OnError(callback func()) {}
func (f *fakeNonMutableViewContext) StartSpanFrom(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}
