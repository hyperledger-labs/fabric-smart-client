/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commondig "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	viewmock "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	p2pmock "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/p2p/mock"
	viewapi "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// fakeDrain is a drainState whose drain the test starts with drain().
type fakeDrain struct {
	mu       sync.Mutex
	draining bool
	onDrain  []func()
}

func (f *fakeDrain) Draining() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.draining
}

func (f *fakeDrain) OnDrain(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onDrain = append(f.onDrain, fn)
}

func (f *fakeDrain) drain() {
	f.mu.Lock()
	f.draining = true
	fns := f.onDrain
	f.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// healthGRPCServer is a fakeGRPCServer that records SetNotServing.
type healthGRPCServer struct {
	*fakeGRPCServer
	notServing bool
}

func (h *healthGRPCServer) SetNotServing() { h.notServing = true }

// handlerServer is a fakeServer that keeps the handlers registered on it.
type handlerServer struct {
	*fakeServer
	mu       sync.Mutex
	handlers map[string]http.Handler
}

func newHandlerServer() *handlerServer {
	return &handlerServer{fakeServer: newFakeServer(), handlers: map[string]http.Handler{}}
}

func (h *handlerServer) RegisterHandler(path string, handler http.Handler, _ bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[path] = handler
}

func (h *handlerServer) get(t *testing.T, path string) int {
	t.Helper()
	h.mu.Lock()
	handler := h.handlers[path]
	h.mu.Unlock()
	require.NotNil(t, handler, path)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code
}

// startSDK starts a view SDK with deps and returns it.
func startSDK(t *testing.T, deps sdkDeps) *SDK {
	t.Helper()
	c := NewContainer()
	registry := view.NewServiceProvider()
	cfg := providerFrom(t, "")
	require.NoError(t, registry.RegisterService(cfg))
	s := NewSDKFrom(commondig.NewBaseSDK(c, cfg), registry)

	sess := &viewmock.Session{}
	sess.ReceiveReturns(make(chan *viewapi.Message))
	commMock := &p2pmock.CommLayer{}
	commMock.MasterSessionReturns(sess, nil)
	tp, err := newTracerProvider(&disabled.Provider{}, cfg)
	require.NoError(t, err)
	wireSDKDeps(t, c, tp, commMock, deps)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, s.Start(ctx))
	return s
}

func TestSDK_DrainSetsHealthNotServingAndGate(t *testing.T) {
	t.Parallel()
	d := &fakeDrain{}
	gs := &healthGRPCServer{fakeGRPCServer: &fakeGRPCServer{}}
	ws := newHandlerServer()
	s := startSDK(t, sdkDeps{drain: d, grpcServer: gs, webServer: ws})
	t.Cleanup(func() { _ = s.Stop() })

	require.Equal(t, http.StatusOK, ws.get(t, "/readyz"))
	d.drain()
	require.True(t, gs.notServing)
	require.Equal(t, http.StatusServiceUnavailable, ws.get(t, "/readyz"))
	require.Equal(t, http.StatusOK, ws.get(t, "/healthz"))
}

// The KVS outlives the teardown of every SDK installed after the view SDK: Node.Stop stops
// those first, and the view SDK closes the KVS in its own Stop.
func TestSDK_StopOrder(t *testing.T) {
	t.Parallel()
	p := providerFrom(t, "")
	kvsInst, err := newKVS(p, newMuxDriver(p, mem.NewNamedDriver(sqlite.NewDbProvider())))
	require.NoError(t, err)
	s := startSDK(t, sdkDeps{kvs: kvsInst})

	// A network provider stopping after the node context was cancelled still writes.
	require.NoError(t, kvsInst.Put(context.Background(), "k", "v"))
	require.NoError(t, s.Stop())
	require.Error(t, kvsInst.Put(context.Background(), "k", "v"), "Stop closes the KVS")
	require.NoError(t, s.Stop(), "a second Stop does nothing")
}

func TestSDK_StopBeforeStart(t *testing.T) {
	t.Parallel()
	c := NewContainer()
	registry := view.NewServiceProvider()
	cfg := providerFrom(t, "")
	require.NoError(t, registry.RegisterService(cfg))
	require.NoError(t, NewSDKFrom(commondig.NewBaseSDK(c, cfg), registry).Stop())
}

func TestDrainGate(t *testing.T) {
	t.Parallel()
	d := &fakeDrain{}
	g := drainGate{Service: &fakeViewService{}, state: d}

	_, reason := g.NotReady()
	require.False(t, reason)
	_, err := g.ProcessCommand(t.Context(), &protos.SignedCommand{})
	require.Equal(t, codes.Unimplemented, status.Code(err), "passes through before the drain")

	d.drain()
	r, notReady := g.NotReady()
	require.True(t, notReady)
	require.Equal(t, "draining", r)
	_, err = g.ProcessCommand(t.Context(), &protos.SignedCommand{})
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.Equal(t, codes.Unavailable, status.Code(g.StreamCommand(nil)))
}
