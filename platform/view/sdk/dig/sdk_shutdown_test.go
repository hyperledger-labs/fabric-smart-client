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
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commondig "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
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

func (f *fakeDrain) TryTrack() (func(), bool) {
	if f.Draining() {
		return nil, false
	}
	return func() {}, true
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

// networkSDK wraps the view SDK as the Fabric SDK does: its Stop stops a network provider,
// which writes to the KVS, and then the SDK it wraps.
type networkSDK struct {
	commondig.SDK
	kvs      *kvs.KVS
	writeErr error
}

func (n *networkSDK) Stop() error {
	n.writeErr = n.kvs.Put(context.Background(), "flushed", "v")
	return n.SDK.Stop()
}

// Node.Stop cancels the node context and then calls Stop on the outermost SDK. A network
// provider that writes to the KVS while it stops finds it open, because the view SDK closes the
// KVS only in its own Stop, which the wrapper calls last.
func TestSDK_StopOrder(t *testing.T) {
	t.Parallel()
	p := providerFrom(t, "")
	kvsInst, err := newKVS(p, newMuxDriver(p, mem.NewNamedDriver(sqlite.NewDbProvider())))
	require.NoError(t, err)

	c := NewContainer()
	registry := view.NewServiceProvider()
	require.NoError(t, registry.RegisterService(p))
	viewSDK := NewSDKFrom(commondig.NewBaseSDK(c, p), registry)
	outer := &networkSDK{SDK: viewSDK, kvs: kvsInst}
	sess := &viewmock.Session{}
	sess.ReceiveReturns(make(chan *viewapi.Message))
	commMock := &p2pmock.CommLayer{}
	commMock.MasterSessionReturns(sess, nil)
	tp, err := newTracerProvider(&disabled.Provider{}, p)
	require.NoError(t, err)
	wireSDKDeps(t, c, tp, commMock, sdkDeps{kvs: kvsInst})

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, outer.Start(ctx))
	cancel() // as Node.Stop does before the teardown
	require.NoError(t, outer.Stop())

	require.NoError(t, outer.writeErr, "the KVS is open while the network provider stops")
	require.Error(t, kvsInst.Put(context.Background(), "k", "v"), "the view SDK closes the KVS last")
	require.NoError(t, viewSDK.Stop(), "a second Stop does nothing")
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

// closeHost is a P2P host that records Close, which the comm layer calls when it stops.
type closeHost struct {
	host.P2PHost
	closed chan struct{}
	once   sync.Once
}

func (*closeHost) PeerID() host.PeerID                        { return "me" }
func (*closeHost) Start(func(host.P2PStream)) error           { return nil }
func (*closeHost) Wait()                                      {}
func (h *closeHost) Close() error                             { h.once.Do(func() { close(h.closed) }); return nil }
func (h *closeHost) GetNewHost() (host.P2PHost, error)        { return h, nil }
func (*closeHost) StreamHash(host.StreamInfo) host.StreamHash { return "" }

// The comm layer outlives the node context: responders cancelled with it can still reply, and
// only Stop, after the servers, closes the host.
func TestSDK_CommStopsInStopNotOnCancel(t *testing.T) {
	t.Parallel()
	h := &closeHost{closed: make(chan struct{})}
	c := NewContainer()
	registry := view.NewServiceProvider()
	cfg := providerFrom(t, "")
	commSvc, err := comm.NewService(h, nil, cfg, &disabled.Provider{})
	require.NoError(t, err)
	require.NoError(t, registry.RegisterService(cfg))
	s := NewSDKFrom(commondig.NewBaseSDK(c, cfg), registry)
	sess := &viewmock.Session{}
	sess.ReceiveReturns(make(chan *viewapi.Message))
	commMock := &p2pmock.CommLayer{}
	commMock.MasterSessionReturns(sess, nil)
	tp, err := newTracerProvider(&disabled.Provider{}, cfg)
	require.NoError(t, err)
	wireSDKDeps(t, c, tp, commMock, sdkDeps{comm: commSvc})

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, s.Start(ctx))
	require.Eventually(t, func() bool { _, err := commSvc.MasterSession(); return err == nil }, 5*time.Second, 10*time.Millisecond, "comm initializes")

	cancel()
	select {
	case <-h.closed:
		t.Fatal("the node context closed the comm host before Stop")
	case <-time.After(200 * time.Millisecond):
	}
	require.NoError(t, s.Stop())
	select {
	case <-h.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not close the comm host")
	}
}
