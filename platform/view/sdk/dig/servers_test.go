/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/operations"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// fakeServer is a minimal Server implementation whose Start blocks until
// Stop is called, so tests can observe whether Serve/serve waits for the
// server goroutine to actually finish before returning.
type fakeServer struct {
	stopCh    chan struct{}
	closeOnce sync.Once
	stoppedFl atomic.Bool
}

func newFakeServer() *fakeServer {
	return &fakeServer{
		stopCh: make(chan struct{}),
	}
}

func (*fakeServer) RegisterHandler(_ string, _ http.Handler, _ bool) {}

func (f *fakeServer) Start() error {
	<-f.stopCh
	return nil
}

func (f *fakeServer) Stop() error {
	f.closeOnce.Do(func() {
		f.stoppedFl.Store(true)
		close(f.stopCh)
	})
	return nil
}

func (f *fakeServer) stopped() bool {
	return f.stoppedFl.Load()
}

func TestServe_WaitsForServersOnShutdown(t *testing.T) { //nolint:paralleltest // relies on server-goroutine shutdown timing; must run serially
	ctx, cancel := context.WithCancel(context.Background())
	ws := newFakeServer() // Start blocks until Stop
	// Zero OperationsServer: the endpoints share the web listener, so startServe starts none.
	wg := startServe(nil, ws, OperationsServer{}, nil, nil, ctx)
	cancel()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not join server goroutines on shutdown")
	}
	require.True(t, ws.stopped())
}

func TestCheckTLSConfig(t *testing.T) {
	t.Parallel()

	t.Run("rejected removed keys", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  metrics:
    prometheus:
      tls: true
`)
		err := CheckTLSConfig(p)
		require.ErrorContains(t, err, "configuration key [fsc.metrics.prometheus.tls] has been removed")
	})

	t.Run("invalid grpc tls", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  grpc:
    enabled: true
    tls:
      clientRootCAs:
        files:
          - non-existent-ca.crt
`)
		err := CheckTLSConfig(p)
		require.ErrorContains(t, err, "invalid fsc.grpc TLS configuration")
	})

	t.Run("invalid web tls", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  grpc:
    enabled: false
  web:
    enabled: true
    tls:
      clientRootCAs:
        files:
          - non-existent-ca.crt
`)
		err := CheckTLSConfig(p)
		require.ErrorContains(t, err, "invalid fsc.web TLS configuration")
	})

	t.Run("both disabled succeeds", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  grpc:
    enabled: false
  web:
    enabled: false
`)
		require.NoError(t, CheckTLSConfig(p))
	})

	t.Run("both enabled with valid tls succeeds", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  tls:
    enabled: true
    cert:
      file: server.crt
    key:
      file: server.key
  grpc:
    enabled: true
    address: 127.0.0.1:0
  web:
    enabled: true
    address: 127.0.0.1:0
`)
		require.NoError(t, CheckTLSConfig(p))
	})
}

type fakeViewManager struct{}

func (*fakeViewManager) NewView(string, []byte) (view.View, error) { return nil, nil }
func (*fakeViewManager) InitiateView(context.Context, view.View) (any, error) {
	return nil, nil
}

func (*fakeViewManager) InitiateContext(context.Context, view.View) (view.Context, error) {
	return nil, nil
}
func (*fakeViewManager) DeleteContext(string) {}

type fakeIdentityProvider struct{}

func (*fakeIdentityProvider) DefaultIdentity() view.Identity { return nil }
func (*fakeIdentityProvider) Admins() []view.Identity        { return nil }
func (*fakeIdentityProvider) Clients() []view.Identity       { return nil }

func TestNewWebServer(t *testing.T) {
	t.Parallel()

	vm := &fakeViewManager{}
	ip := &fakeIdentityProvider{}
	tp, err := newTracerProvider(&disabled.Provider{}, providerFrom(t, ""))
	require.NoError(t, err)

	t.Run("disabled returns dummy server", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  web:
    enabled: false
`)
		ws, err := NewWebServer(p, vm, ip, tp.Default)
		require.NoError(t, err)
		require.NotNil(t, ws)
	})

	t.Run("invalid tls fails", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  web:
    enabled: true
    address: 127.0.0.1:0
    tls:
      clientRootCAs:
        files:
          - non-existent.crt
`)
		_, err := NewWebServer(p, vm, ip, tp.Default)
		require.ErrorContains(t, err, "failed resolving fsc.web.tls")
	})

	t.Run("enabled with valid config succeeds", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  web:
    enabled: true
    address: 127.0.0.1:0
`)
		ws, err := NewWebServer(p, vm, ip, tp.Default)
		require.NoError(t, err)
		require.NotNil(t, ws)
	})
}

func TestNewOperationsOptionsAndLogger(t *testing.T) {
	t.Parallel()

	p := providerFrom(t, `
fsc:
  metrics:
    provider: prometheus
    clientAuthRequired: true
`)
	opts, err := NewOperationsOptions(p)
	require.NoError(t, err)
	require.NotNil(t, opts)
	require.Equal(t, "prometheus", opts.Metrics.Provider)
	require.Equal(t, "1.0.0", opts.Version)
	require.True(t, opts.RequireClientCert)

	opsLogger := NewOperationsLogger(opts)
	require.NotNil(t, opsLogger)
}

func TestNewGRPCServer(t *testing.T) {
	t.Parallel()

	t.Run("disabled returns nil", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  grpc:
    enabled: false
`)
		srv, err := NewGRPCServer(p)
		require.NoError(t, err)
		require.Nil(t, srv)
	})

	t.Run("invalid tls config returns error", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  grpc:
    enabled: true
    address: 127.0.0.1:0
    tls:
      clientRootCAs:
        files:
          - non-existent.crt
`)
		_, err := NewGRPCServer(p)
		require.ErrorContains(t, err, "failed resolving fsc.grpc.tls")
	})

	t.Run("enabled with valid config succeeds", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  grpc:
    enabled: true
    address: 127.0.0.1:0
`)
		srv, err := NewGRPCServer(p)
		require.NoError(t, err)
		require.NotNil(t, srv)
		defer srv.Stop()
	})
}

func TestNewServerConfig_KeepAlive(t *testing.T) {
	t.Parallel()

	t.Run("valid keepalive config", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  grpc:
    connectionTimeout: 10s
    keepalive:
      time: 1m
      timeout: 20s
      max-connection-idle: 30s
`)
		cfg, err := NewServerConfig(p)
		require.NoError(t, err)
		require.Equal(t, 10*time.Second, cfg.ConnectionTimeout)
		require.NotNil(t, cfg.KeepAliveConfig)
		require.Equal(t, time.Minute, cfg.KeepAliveConfig.Time)
		require.Equal(t, 20*time.Second, cfg.KeepAliveConfig.Timeout)
		require.Equal(t, 30*time.Second, cfg.KeepAliveConfig.MaxConnectionIdle)
	})

	t.Run("invalid keepalive config unmarshal error", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  grpc:
    keepalive: "invalid-string-should-be-map"
`)
		_, err := NewServerConfig(p)
		require.ErrorContains(t, err, "error unmarshalling keep alive config")
	})
}

func TestNewViewServiceServer(t *testing.T) {
	t.Parallel()

	tp, err := newTracerProvider(&disabled.Provider{}, providerFrom(t, ""))
	require.NoError(t, err)

	t.Run("with nil grpcServer", func(t *testing.T) {
		t.Parallel()
		srv, err := NewViewServiceServer(nil, nil, nil, tp.Default, nil)
		require.NoError(t, err)
		require.NotNil(t, srv)
	})

	t.Run("with non-nil grpcServer", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  tls:
    enabled: true
    cert:
      file: server.crt
    key:
      file: server.key
  grpc:
    enabled: true
    address: 127.0.0.1:0
    tls:
      clientAuthRequired: true
      clientRootCAs:
        files:
          - ca.crt
`)
		grpcServer, err := NewGRPCServer(p)
		require.NoError(t, err)
		require.NotNil(t, grpcServer)
		defer grpcServer.Stop()

		srv, err := NewViewServiceServer(nil, nil, nil, tp.Default, grpcServer)
		require.NoError(t, err)
		require.NotNil(t, srv)
	})
}

func TestServe_FullLifecycle(t *testing.T) { //nolint:paralleltest // relies on server lifecycle and goroutines
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ws := newFakeServer()
	ownOps := newFakeServer()
	ops := OperationsServer{
		Server: ws,
		Own:    ownOps,
	}

	opsSystem, err := operations.NewOperationSystem(ws, NewOperationsLogger(&operations.Options{}), &disabled.Provider{}, &operations.Options{Version: "1.0.0"})
	require.NoError(t, err)

	p := providerFrom(t, "")
	namedDriver := mem.NewNamedDriver(sqlite2.NewDbProvider())
	muxDriver := newMuxDriver(p, namedDriver)
	kvsInst, err := newKVS(p, muxDriver)
	require.NoError(t, err)

	wg := startServe(nil, ws, ops, opsSystem, kvsInst, ctx)
	// Cancel context to initiate shutdown
	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("startServe did not join all server goroutines in time")
	}

	require.True(t, ws.stopped())
	require.True(t, ownOps.stopped())

	// Serve is the exported entrypoint that wraps startServe. Exercise it with its
	// own server and context, and assert it stops the server and returns after
	// cancellation rather than blocking.
	serveCtx, serveCancel := context.WithCancel(context.Background())
	serveWS := newFakeServer()
	served := make(chan struct{})
	go func() {
		Serve(nil, serveWS, OperationsServer{}, nil, nil, serveCtx)
		close(served)
	}()
	serveCancel()
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after context cancellation")
	}
	select {
	case <-serveWS.stopCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop server after context cancellation")
	}
	require.True(t, serveWS.stopped())
}
