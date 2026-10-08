/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/operations"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	web "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
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
    tls:
      clientRootCAs:
        files:
          - ca.crt
`)
		require.NoError(t, CheckTLSConfig(p))
	})

	for _, tc := range []struct{ name, metrics, wantErr string }{
		{name: "explicit operations client auth the listener cannot verify fails", metrics: "    clientAuthRequired: true\n", wantErr: "fsc.web.tls listener never requests a client certificate"},
		{name: "defaulted operations client auth the listener cannot verify succeeds"},
		{name: "unresolvable metrics tls fails", metrics: "    clientAuthRequired: false\n    address: 127.0.0.1:0\n    tls:\n      enabled: true\n      cert:\n        file: missing.crt\n      key:\n        file: missing.key\n", wantErr: "invalid fsc.metrics configuration: failed resolving fsc.metrics.tls"},
		{name: "metrics tls without an address fails", metrics: "    clientAuthRequired: false\n    tls:\n      enabled: false\n", wantErr: "fsc.metrics.tls has no effect without fsc.metrics.address"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := providerFrom(t, "fsc:\n  web:\n    enabled: true\n    address: 127.0.0.1:0\n  metrics:\n"+tc.metrics)
			if tc.wantErr == "" {
				require.NoError(t, CheckTLSConfig(p))
				return
			}
			require.ErrorContains(t, CheckTLSConfig(p), tc.wantErr)
		})
	}
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

	for _, tc := range []struct {
		name       string
		clientAuth string
		want       bool
	}{
		{name: "absent requires a client certificate", want: true},
		{name: "explicit true", clientAuth: "\n    clientAuthRequired: true", want: true},
		{name: "explicit false opts out", clientAuth: "\n    clientAuthRequired: false", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := providerFrom(t, "fsc:\n  metrics:\n    provider: prometheus"+tc.clientAuth+"\n")
			opts, err := NewOperationsOptions(p)
			require.NoError(t, err)
			require.NotNil(t, opts)
			require.Equal(t, "prometheus", opts.Metrics.Provider)
			require.Equal(t, "1.0.0", opts.Version)
			require.Equal(t, tc.want, opts.RequireClientCert)
			require.NotNil(t, NewOperationsLogger(opts))
		})
	}
}

// A value that is not a boolean must not disable the check: null, an empty string and
// YAML 1.1 words like yes/on all read as false through GetBool.
func TestNewOperationsOptionsRejectsNonBoolClientAuth(t *testing.T) {
	t.Parallel()

	for _, v := range []string{"", "null", "~", `""`, "yes", "on", "ture", "[true]"} {
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			p := providerFrom(t, "fsc:\n  metrics:\n    clientAuthRequired: "+v+"\n")
			_, err := NewOperationsOptions(p)
			require.ErrorContains(t, err, "invalid fsc.metrics.clientAuthRequired")
		})
	}
}

// A listener that never requests a client certificate makes a required one unsatisfiable:
// an explicit true is an error, a defaulted one a warning naming the endpoints it closes.
func TestOperationsClientAuthOnListener(t *testing.T) {
	t.Parallel()

	const (
		tlsOn   = "  tls:\n    enabled: true\n    cert:\n      file: server.crt\n    key:\n      file: server.key\n"
		webOn   = "  web:\n    enabled: true\n    address: 127.0.0.1:0\n"
		rootCAs = "    tls:\n      clientRootCAs:\n        files:\n          - ca.crt\n"
		on      = "  metrics:\n    clientAuthRequired: true\n"
	)
	for _, tc := range []struct {
		name, yaml, wantWarn, wantErr string
		want                          bool
	}{
		{name: "plaintext web listener", yaml: webOn, want: true, wantWarn: "defaults to true, but the fsc.web.tls listener never requests a client certificate, so /logspec reject"},
		{name: "plaintext web listener, prometheus", yaml: webOn + "  metrics:\n    provider: prometheus\n", want: true, wantWarn: "so /metrics and /logspec reject"},
		{name: "plaintext web listener with client root CAs", yaml: webOn + "    tls:\n      enabled: false\n      clientRootCAs:\n        files:\n          - ca.crt\n", want: true, wantWarn: "fsc.web.tls listener never requests"},
		{name: "tls web listener without client root CAs", yaml: tlsOn + webOn, want: true, wantWarn: "fsc.web.tls listener never requests"},
		{name: "plaintext web listener, explicit true", yaml: webOn + on, wantErr: "fsc.metrics.clientAuthRequired is true, but the fsc.web.tls listener never requests a client certificate, so /logspec would reject"},
		{name: "tls web listener without client root CAs, explicit true", yaml: tlsOn + webOn + on, wantErr: "fsc.web.tls listener never requests"},
		{name: "tls web listener with client root CAs", yaml: tlsOn + webOn + rootCAs, want: true},
		{name: "tls web listener with client root CAs, explicit true", yaml: tlsOn + webOn + rootCAs + on, want: true},
		{name: "plaintext web listener, explicit false", yaml: webOn + "  metrics:\n    clientAuthRequired: false\n"},
		{name: "no listener", yaml: "  web:\n    enabled: false\n", want: true},
		{name: "plaintext metrics listener", yaml: tlsOn + webOn + rootCAs + "  metrics:\n    address: 127.0.0.1:0\n    tls:\n      enabled: false\n", want: true, wantWarn: "fsc.metrics.tls listener never requests"},
		{name: "plaintext metrics listener, explicit true", yaml: tlsOn + webOn + rootCAs + on + "    address: 127.0.0.1:0\n    tls:\n      enabled: false\n", wantErr: "fsc.metrics.tls listener never requests"},
		{name: "metrics listener with client root CAs", yaml: tlsOn + "  metrics:\n    address: 127.0.0.1:0\n" + rootCAs, want: true},
		{name: "unresolvable listener TLS", yaml: "  tls:\n    enabled: true\n" + webOn, wantErr: "failed resolving fsc.web.tls"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := providerFrom(t, "fsc:\n"+tc.yaml)
			got, warning, err := operationsClientAuth(p)
			opts, optsErr := NewOperationsOptions(p)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.ErrorContains(t, optsErr, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.NoError(t, optsErr)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.want, opts.RequireClientCert)
			if tc.wantWarn == "" {
				require.Empty(t, warning)
			} else {
				require.Contains(t, warning, tc.wantWarn)
			}
		})
	}
}

// mtlsWeb is a web listener that requests client certificates signed by ca.crt.
const mtlsWeb = "  web:\n    enabled: true\n    tls:\n      enabled: true\n      cert:\n        file: server.crt\n      key:\n        file: server.key\n      clientRootCAs:\n        files:\n          - ca.crt\n"

// TestOperationsLogspecClientAuth sends a PUT /logspec to an operations system wired from
// configuration: without a client certificate it is rejected unless
// fsc.metrics.clientAuthRequired is explicitly false.
func TestOperationsLogspecClientAuth(t *testing.T) { //nolint:paralleltest // mutates the process-wide flogging.Global spec
	original := flogging.Global.Spec()
	t.Cleanup(func() { _ = flogging.Global.ActivateSpec(original) })
	const injected = "fatal"

	for _, tc := range []struct { //nolint:paralleltest // subtests share flogging.Global
		name        string
		config      string
		clientCert  bool
		wantStatus  int
		wantApplied bool
	}{
		{
			name:       "absent, on a listener that requests client certificates",
			config:     mtlsWeb,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:        "absent, client presents a certificate",
			config:      mtlsWeb,
			clientCert:  true,
			wantStatus:  http.StatusNoContent,
			wantApplied: true,
		},
		{
			name:       "absent, on a plaintext listener",
			config:     "  web:\n    enabled: true\n",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:        "explicit false, on a plaintext listener",
			config:      "  web:\n    enabled: true\n  metrics:\n    clientAuthRequired: false\n",
			wantStatus:  http.StatusNoContent,
			wantApplied: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, ca := providerWithCA(t, "fsc:\n"+tc.config)
			// The baseline is taken after providerFrom, whose logging.Init resets the spec.
			baseline := flogging.Global.Spec()
			require.NotEqual(t, injected, baseline)
			opts, err := NewOperationsOptions(p)
			require.NoError(t, err)
			tlsOpts, err := resolveWebTLS(p)
			require.NoError(t, err)
			srv := web.NewServer(web.Options{ListenAddress: "127.0.0.1:0", TLS: tlsOpts})
			require.NoError(t, srv.Start())
			t.Cleanup(func() { _ = srv.Stop() })
			_, err = operations.NewOperationSystem(srv, NewOperationsLogger(opts), &disabled.Provider{}, opts)
			require.NoError(t, err)

			// The client trusts the server and presents a certificate only when clientCert is set.
			scheme, client := "http", http.DefaultClient
			if tlsOpts.UseTLS {
				roots := x509.NewCertPool()
				require.True(t, roots.AppendCertsFromPEM(tlsOpts.ClientRootCAs[0]), "ca.crt signs the server certificate too")
				cfg := &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
				if tc.clientCert {
					kp, err := ca.NewClientCertKeyPair()
					require.NoError(t, err)
					cert, err := tls.X509KeyPair(kp.Cert, kp.Key)
					require.NoError(t, err)
					cfg.Certificates = []tls.Certificate{cert}
				}
				scheme = "https"
				client = &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}
			}
			req, err := http.NewRequest(http.MethodPut, scheme+"://"+srv.Addr()+"/logspec", bytes.NewBufferString(`{"spec":"`+injected+`"}`))
			require.NoError(t, err)
			resp, err := client.Do(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, tc.wantStatus, resp.StatusCode)
			want := baseline
			if tc.wantApplied {
				want = injected
			}
			require.Equal(t, want, flogging.Global.Spec())
		})
	}
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
