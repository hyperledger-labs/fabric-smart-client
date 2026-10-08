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
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	servicesmock "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	server2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestDispatcher(t *testing.T) {
	t.Parallel()
	h := server2.NewHttpHandler()
	d := newDispatcher(h)

	// vc is nil
	req, _ := http.NewRequest(http.MethodPut, "/v1/Views/fid", nil)
	req.SetPathValue("View", "fid")
	reqctx := &server2.ReqContext{
		Req:   req,
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
	req, _ := http.NewRequest(http.MethodPut, "/v1/Views/fid", nil)
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
	require.JSONEq(t, string(expectedJSON), string(resp.(*protos.CommandResponse_CallViewResponse).CallViewResponse.Result))

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

// streamCall runs one streamed call of view vid through c. It returns the Output the
// client received, if any, the error of StreamCallView, and the error that ended the
// client's reads once StreamCallView has returned.
func streamCall(t *testing.T, c *client, vid string) (out []byte, callErr, readErr error) {
	t.Helper()
	errc := make(chan error, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errc <- c.StreamCallView(vid, w, r)
	}))
	t.Cleanup(ts.Close)
	ws, resp, err := websocket.DefaultDialer.Dial("ws"+ts.URL[4:], nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	t.Cleanup(func() { _ = ws.Close() })
	in, err := json.Marshal(server2.Input{Raw: []byte("input")})
	require.NoError(t, err)
	require.NoError(t, ws.WriteMessage(websocket.TextMessage, in))

	// Read to the close before waiting for the handler: answering the close frame ends the
	// server's drain.
	require.NoError(t, ws.SetReadDeadline(time.Now().Add(5*time.Second)))
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			return out, <-errc, err
		}
		var o server2.Output
		require.NoError(t, json.Unmarshal(msg, &o))
		out = o.Raw
	}
}

func TestClientStreamCallView(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		vm      *fakeViewManager
		wantOut string
		wantErr string
		deleted []string
	}{
		{name: "success", vm: &fakeViewManager{}, wantOut: "result", deleted: []string{"ctx-id"}},
		{name: "non-byte result", vm: &fakeViewManager{ctxResult: map[string]string{"status": "ok"}}, wantOut: `{"status":"ok"}`, deleted: []string{"ctx-id"}},
		{name: "RunView failure", vm: &fakeViewManager{ctxRunErr: fmt.Errorf("run view error")}, wantErr: "run view error", deleted: []string{"ctx-id"}},
		{name: "PutService failure", vm: &fakeViewManager{putServiceErr: fmt.Errorf("put service error")}, wantErr: "registering stream command server", deleted: []string{"ctx-id"}},
		{name: "non-mutable context", vm: &fakeViewManager{nonMutableCtx: true}, wantErr: "expected a mutable context", deleted: []string{"ctx-id"}},
		{name: "NewView failure", vm: &fakeViewManager{err: fmt.Errorf("new view error")}, wantErr: "new view error"},
		{name: "InitiateContext failure", vm: &fakeViewManager{initCtxErr: fmt.Errorf("init context error")}, wantErr: "init context error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err, readErr := streamCall(t, newViewClient(tc.vm, &fakeIdentityProvider{}, noop.NewTracerProvider()), "fid")
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				// The close frame carries the failure to the client.
				require.True(t, websocket.IsCloseError(readErr, websocket.CloseInternalServerErr), "websocket not closed: %v", readErr)
				require.ErrorContains(t, readErr, tc.wantErr)
			} else {
				require.NoError(t, err)
				require.True(t, websocket.IsCloseError(readErr, websocket.CloseNormalClosure), "websocket not closed: %v", readErr)
			}
			require.Equal(t, tc.wantOut, string(out))
			require.Equal(t, tc.deleted, tc.vm.deleted)
		})
	}
}

// recordingView records the ID of the context it runs in and fails with err.
type recordingView struct {
	id  string
	err error
}

func (v *recordingView) Call(ctx view2.Context) (any, error) {
	v.id = ctx.ID()
	return nil, v.err
}

type recordingViewFactory struct{ v *recordingView }

func (f *recordingViewFactory) NewView([]byte) (view2.View, error) { return f.v, nil }

func TestClientStreamCallViewReleasesManagerContext(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		err     error
		wantErr string
	}{
		{name: "success"},
		{name: "view failure", err: fmt.Errorf("view error"), wantErr: "view error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ip := &mock.IdentityProvider{}
			ip.DefaultIdentityReturns(view2.Identity("me"))
			registry := view.NewRegistry()
			metrics := view.NewMetrics(&disabled.Provider{})
			cf := view.NewContextFactory(&servicesmock.ServiceProvider{}, &mock.SessionFactory{}, &mock.EndpointService{}, ip, registry, noop.NewTracerProvider(), metrics, &mock.LocalIdentityChecker{})
			manager := view.NewManager(ip, registry, metrics, cf, view.NewDefaultRunner())
			v := &recordingView{err: tc.err}
			require.NoError(t, manager.RegisterFactory("recording", &recordingViewFactory{v: v}))
			c := newViewClient(manager, &fakeIdentityProvider{}, noop.NewTracerProvider())

			_, err, readErr := streamCall(t, c, "recording")
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				// The close frame carries the failure to the client.
				require.True(t, websocket.IsCloseError(readErr, websocket.CloseInternalServerErr), "websocket not closed: %v", readErr)
				require.ErrorContains(t, readErr, tc.wantErr)
			} else {
				require.NoError(t, err)
				require.True(t, websocket.IsCloseError(readErr, websocket.CloseNormalClosure), "websocket not closed: %v", readErr)
			}
			require.NotEmpty(t, v.id)
			_, err = manager.Context(v.id)
			require.ErrorIs(t, err, view.ErrContextNotFound)
		})
	}
}

func TestViewCallFunc(t *testing.T) {
	t.Parallel()
	f := viewCallFunc(func(_ *server2.ReqContext, _ string, _ []byte) (any, error) {
		return "result", nil
	})
	res, err := f.CallView(nil, "vid", nil)
	require.NoError(t, err)
	require.Equal(t, "result", res)
}

type fakeViewCaller struct {
	err error
}

func (f *fakeViewCaller) CallView(_ *server2.ReqContext, _ string, _ []byte) (any, error) {
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
	deleted       []string
}

func (f *fakeViewManager) NewView(_ string, _ []byte) (view2.View, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &fakeView{}, nil
}

func (f *fakeViewManager) InitiateView(_ context.Context, _ view2.View) (any, error) {
	if f.initErr != nil {
		return nil, f.initErr
	}
	if f.result != nil {
		return f.result, nil
	}
	return []byte("result"), nil
}

func (f *fakeViewManager) InitiateContext(_ context.Context, _ view2.View) (view2.Context, error) {
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

func (f *fakeViewManager) DeleteContext(contextID string) {
	f.deleted = append(f.deleted, contextID)
}

type fakeView struct{}

func (*fakeView) Call(_ view2.Context) (any, error) {
	return nil, nil
}

type fakeIdentityProvider struct{}

func (*fakeIdentityProvider) DefaultIdentity() view2.Identity { return nil }
func (*fakeIdentityProvider) Clients() []view2.Identity       { return nil }

type fakeViewContext struct {
	runErr        error
	putServiceErr error
	result        any
}

func (*fakeViewContext) ID() string { return "ctx-id" }
func (f *fakeViewContext) RunView(_ view2.View, _ ...view2.RunViewOption) (any, error) {
	return f.result, f.runErr
}
func (*fakeViewContext) Context() context.Context      { return context.Background() }
func (*fakeViewContext) GetService(_ any) (any, error) { return nil, nil }
func (*fakeViewContext) Me() view2.Identity            { return nil }
func (*fakeViewContext) IsMe(_ view2.Identity) bool    { return false }
func (*fakeViewContext) Initiator() view2.View         { return nil }
func (*fakeViewContext) GetSession(_ view2.View, _ view2.Identity, _ ...view2.View) (view2.Session, error) {
	return nil, nil
}

func (*fakeViewContext) GetSessionByID(_ string, _ view2.Identity) (view2.Session, error) {
	return nil, nil
}
func (*fakeViewContext) Session() view2.Session { return nil }
func (*fakeViewContext) OnError(_ func())       {}
func (*fakeViewContext) StartSpanFrom(ctx context.Context, _ string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}
func (*fakeViewContext) ResetSessions() error { return nil }
func (f *fakeViewContext) PutService(_ any) error {
	return f.putServiceErr
}

type fakeNonMutableViewContext struct{}

func (*fakeNonMutableViewContext) ID() string { return "ctx-id" }
func (*fakeNonMutableViewContext) RunView(_ view2.View, _ ...view2.RunViewOption) (any, error) {
	return []byte("result"), nil
}
func (*fakeNonMutableViewContext) Context() context.Context      { return context.Background() }
func (*fakeNonMutableViewContext) GetService(_ any) (any, error) { return nil, nil }
func (*fakeNonMutableViewContext) Me() view2.Identity            { return nil }
func (*fakeNonMutableViewContext) IsMe(_ view2.Identity) bool    { return false }
func (*fakeNonMutableViewContext) Initiator() view2.View         { return nil }
func (*fakeNonMutableViewContext) GetSession(_ view2.View, _ view2.Identity, _ ...view2.View) (view2.Session, error) {
	return nil, nil
}

func (*fakeNonMutableViewContext) GetSessionByID(_ string, _ view2.Identity) (view2.Session, error) {
	return nil, nil
}
func (*fakeNonMutableViewContext) Session() view2.Session { return nil }
func (*fakeNonMutableViewContext) OnError(_ func())       {}
func (*fakeNonMutableViewContext) StartSpanFrom(ctx context.Context, _ string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}
