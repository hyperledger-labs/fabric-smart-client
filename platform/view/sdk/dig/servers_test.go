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
	"google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/operations"
	web "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// fakeServer is a WebServer whose Start returns startErr and which records Start and Stop.
type fakeServer struct {
	startErr  error
	startedFl atomic.Bool
	stoppedFl atomic.Bool
}

func newFakeServer() *fakeServer { return &fakeServer{} }

func (*fakeServer) RegisterHandler(_ string, _ http.Handler, _ bool) {}

func (f *fakeServer) Start() error {
	f.startedFl.Store(true)
	return f.startErr
}

func (f *fakeServer) Stop() error {
	f.stoppedFl.Store(true)
	return nil
}

func (f *fakeServer) started() bool { return f.startedFl.Load() }
func (f *fakeServer) stopped() bool { return f.stoppedFl.Load() }

// servingGRPCServer is a GRPCServer whose Start returns serveErr when it is sent, or, once
// Stop is called, grpc.ErrServerStopped, as a grpc.Server stopped before Serve does.
type servingGRPCServer struct {
	fakeGRPCServer
	serveErr  chan error
	stopOnce  sync.Once
	stopCh    chan struct{}
	stoppedFl atomic.Bool
}

func newServingGRPCServer() *servingGRPCServer {
	return &servingGRPCServer{serveErr: make(chan error), stopCh: make(chan struct{})}
}

func (g *servingGRPCServer) Start() error {
	select {
	case err := <-g.serveErr:
		return err
	case <-g.stopCh:
		return grpc.ErrServerStopped
	}
}

func (g *servingGRPCServer) Stop() {
	g.stopOnce.Do(func() {
		g.stoppedFl.Store(true)
		close(g.stopCh)
	})
}

func TestStartServe_StartFailureReturnsError(t *testing.T) {
	t.Parallel()
	boom := errors.New("address already in use")

	t.Run("web server", func(t *testing.T) {
		t.Parallel()
		ws, own, gs := &fakeServer{startErr: boom}, newFakeServer(), newServingGRPCServer()
		stop, err := startServe(gs, ws, OperationsServer{Own: own}, nil, &grpcStoppedGate{})
		require.ErrorIs(t, err, boom)
		require.ErrorContains(t, err, "failed starting web server")
		require.Nil(t, stop)
		require.False(t, own.started(), "nothing after the failed server starts")
		require.True(t, ws.stopped())
		require.True(t, own.stopped())
		require.True(t, gs.stoppedFl.Load(), "the gRPC listener is released")
	})

	t.Run("metrics server", func(t *testing.T) {
		t.Parallel()
		ws, own := newFakeServer(), &fakeServer{startErr: boom}
		_, err := startServe(nil, ws, OperationsServer{Own: own}, nil, &grpcStoppedGate{})
		require.ErrorIs(t, err, boom)
		require.ErrorContains(t, err, "failed starting metrics server")
		require.True(t, ws.stopped(), "the web server that started is stopped")
	})
}

func TestStartServe_StopsEveryServer(t *testing.T) {
	t.Parallel()
	ws, own, gs := newFakeServer(), newFakeServer(), newServingGRPCServer()
	opsSystem, err := operations.NewOperationSystem(ws, NewOperationsLogger(&operations.Options{}), &disabled.Provider{}, &operations.Options{Version: "1.0.0"})
	require.NoError(t, err)
	gate := &grpcStoppedGate{}

	stop, err := startServe(gs, ws, OperationsServer{WebServer: ws, Own: own}, opsSystem, gate)
	require.NoError(t, err)
	require.True(t, ws.started())
	require.True(t, own.started())
	stop()
	stop() // a second call returns at once

	require.True(t, ws.stopped())
	require.True(t, own.stopped())
	require.True(t, gs.stoppedFl.Load())
	require.Never(t, func() bool { _, notReady := gate.NotReady(); return notReady }, 200*time.Millisecond, 10*time.Millisecond,
		"a gRPC server stopped by stop is not a failure, whatever Start returns")
}

// A gRPC server whose Serve fails while the node runs leaves the process running and makes
// the node report not ready.
func TestStartServe_GRPCFailureReportsNotReady(t *testing.T) {
	t.Parallel()
	gs, gate := newServingGRPCServer(), &grpcStoppedGate{}
	stop, err := startServe(gs, newFakeServer(), OperationsServer{}, nil, gate)
	require.NoError(t, err)
	defer stop()

	gs.serveErr <- errors.New("accept: too many open files")
	require.Eventually(t, func() bool { _, notReady := gate.NotReady(); return notReady }, 5*time.Second, 10*time.Millisecond)
	reason, _ := gate.NotReady()
	require.Equal(t, "grpc server stopped", reason)
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
    address: 127.0.0.1:0
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
    address: 127.0.0.1:0
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
  view:
    remote:
      enabled: true
      backends: [grpc, web]
`)
		require.NoError(t, CheckTLSConfig(p))
	})

	for _, tc := range []struct{ name, address, wantErr string }{
		{name: "grpc enabled without address fails", wantErr: "fsc.grpc.enabled is true but fsc.grpc.address is not set"},
		{name: "grpc enabled with ip only fails", address: "\n    address: 127.0.0.1", wantErr: "invalid fsc.grpc.address [127.0.0.1]"},
		{name: "grpc enabled with port only fails", address: "\n    address: 20000", wantErr: "invalid fsc.grpc.address [20000]"},
		{name: "grpc enabled with ip and port succeeds", address: "\n    address: 127.0.0.1:20000"},
		{name: "grpc enabled with port and no host succeeds", address: "\n    address: :20000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// A node serving gRPC configures the remote view API on it.
			p := providerFrom(t, "fsc:\n  grpc:\n    enabled: true"+tc.address+
				"\n  view:\n    remote:\n      enabled: true\n      backends: [grpc]\n  metrics:\n    clientAuthRequired: false\n")
			if tc.wantErr == "" {
				require.NoError(t, CheckTLSConfig(p))
				return
			}
			require.ErrorContains(t, CheckTLSConfig(p), tc.wantErr)
		})
	}

	t.Run("grpc disabled ignores a malformed address", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, "fsc:\n  grpc:\n    enabled: false\n    address: 20000\n  metrics:\n    clientAuthRequired: false\n")
		require.NoError(t, CheckTLSConfig(p))
	})

	for _, tc := range []struct{ name, address, wantErr string }{
		{name: "web enabled without address fails", wantErr: "fsc.web.address is not set"},
		{name: "web enabled with port only fails", address: "\n    address: 20002", wantErr: "invalid fsc.web.address [20002]"},
		{name: "web enabled with port and no host succeeds", address: "\n    address: :20002"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := providerFrom(t, "fsc:\n  web:\n    enabled: true"+tc.address+"\n  metrics:\n    clientAuthRequired: false\n")
			if tc.wantErr == "" {
				require.NoError(t, CheckTLSConfig(p))
				return
			}
			require.ErrorContains(t, CheckTLSConfig(p), tc.wantErr)
		})
	}

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

	t.Run("enabled without address fails", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  web:
    enabled: true
`)
		_, err := NewWebServer(p)
		require.ErrorContains(t, err, "fsc.web.address is not set")
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
		_, err := NewWebServer(p)
		require.ErrorContains(t, err, "failed resolving fsc.web.tls")
	})

	t.Run("enabled succeeds", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  web:
    enabled: true
    address: 127.0.0.1:0
`)
		ws, err := NewWebServer(p)
		require.NoError(t, err)
		require.NotNil(t, ws)
	})
}

// The probes are served on the web listener without a client certificate, and on any
// listener the SDK holds, including a replaced one.
func TestMountHealth(t *testing.T) {
	t.Parallel()
	cs := connstate.NewMetrics(&disabled.Provider{})
	cs.Tracker("net", "ch", connstate.ComponentDelivery, "peer0:7051").Set(connstate.Connecting)

	ws, err := NewWebServer(providerFrom(t, "fsc:\n  web:\n    enabled: true\n    address: 127.0.0.1:0\n"))
	require.NoError(t, err)
	mountHealth(ws, cs, nil)
	require.NoError(t, ws.Start())
	t.Cleanup(func() { _ = ws.Stop() })
	addr := ws.(*web.Server).Addr()
	for path, want := range map[string]int{
		"/healthz": http.StatusOK,
		"/readyz":  http.StatusServiceUnavailable,
	} {
		resp, err := http.Get("http://" + addr + path)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, want, resp.StatusCode, path)
	}

	replaced := &recordingWebServer{}
	mountHealth(replaced, cs, nil)
	require.Equal(t, map[string]bool{"/healthz": false, "/readyz": false}, replaced.secure)
}

// flipGate is a readiness gate that is not ready until ready is set.
type flipGate struct {
	reason string
	ready  atomic.Bool
}

func (g *flipGate) NotReady() (string, bool) { return g.reason, !g.ready.Load() }

func notReadyGate(reason string) *flipGate { return &flipGate{reason: reason} }

// /readyz reports drain first, then the gates SDKs provide, then connection state; a nil gate is
// skipped. Not parallel: the migrating gate sits between drain and the SDK gates, and a parallel
// test opening a SQLite store would make it report first.
func TestMountHealthReadinessGates(t *testing.T) { //nolint:paralleltest // reads package-level migration state
	readyGate := &flipGate{reason: "app"}
	readyGate.ready.Store(true)
	for _, tc := range []struct { //nolint:paralleltest // see above
		name  string
		drain web.ReadinessGate
		gates []web.ReadinessGate
		want  string // "" is ready
	}{
		{name: "no gates"},
		{name: "app gate not ready", gates: []web.ReadinessGate{notReadyGate("app")}, want: "app"},
		{name: "drain wins", drain: notReadyGate("draining"), gates: []web.ReadinessGate{notReadyGate("app")}, want: "draining"},
		{name: "app gate ready", gates: []web.ReadinessGate{readyGate}},
		{name: "nil gate", gates: []web.ReadinessGate{nil}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := newHandlerServer()
			mountHealth(ws, connstate.NewMetrics(&disabled.Provider{}), tc.drain, tc.gates...)
			require.Equal(t, tc.want, ws.readyReason(t))
		})
	}
}

// A node whose gRPC server stopped serving reports "grpc server stopped", and "draining" once
// it also drains: draining comes first, so a deliberate shutdown is not reported as a gRPC
// failure. Not parallel, for the reason given at TestMountHealthReadinessGates.
func TestSDK_ReadinessGRPCStoppedAfterDraining(t *testing.T) { //nolint:paralleltest // reads package-level migration state
	d := &fakeDrain{}
	gs := newServingGRPCServer()
	ws := newHandlerServer()
	s := startSDK(t, sdkDeps{drain: d, grpcServer: gs, webServer: ws})
	t.Cleanup(func() { _ = s.Stop() })
	require.Empty(t, ws.readyReason(t))

	gs.serveErr <- errors.New("accept: too many open files")
	require.Eventually(t, func() bool { return ws.readyReason(t) == "grpc server stopped" }, 5*time.Second, 10*time.Millisecond)

	d.drain()
	require.Equal(t, "draining", ws.readyReason(t))
}

// A gate provided into the "fsc-readiness-gates" group before Start, as an SDK does from its
// Install, holds /readyz at 503 with its reason until it is ready. Not parallel, for the reason
// given at TestMountHealthReadinessGates.
func TestSDK_ReadinessGateEntry(t *testing.T) { //nolint:paralleltest // reads package-level migration state
	gate := notReadyGate("tms not loaded")
	ws := newHandlerServer()
	s := startSDK(t, sdkDeps{webServer: ws, readinessGates: []web.ReadinessGate{gate}})
	t.Cleanup(func() { _ = s.Stop() })

	require.Equal(t, "tms not loaded", ws.readyReason(t))
	gate.ready.Store(true)
	require.Empty(t, ws.readyReason(t))
}

func TestCheckTLSConfigRejectsNegativeWebMaxRecvMsgSize(t *testing.T) {
	t.Parallel()
	err := CheckTLSConfig(providerFrom(t, "fsc:\n  web:\n    enabled: true\n    address: 127.0.0.1:0\n    maxRecvMsgSize: -1\n"))
	require.ErrorContains(t, err, "fsc.web.maxRecvMsgSize")
}

func TestRemoteViewTimeout(t *testing.T) {
	t.Parallel()
	require.Equal(t, DefaultRemoteViewTimeout, RemoteViewTimeout(providerFrom(t, "")))
	require.Equal(t, 10*time.Second, RemoteViewTimeout(providerFrom(t, "fsc:\n  view:\n    remote:\n      timeout: 10s\n")))
	require.Equal(t, DefaultRemoteViewTimeout, RemoteViewTimeout(providerFrom(t, "fsc:\n  view:\n    remote:\n      timeout: 0s\n")))
}

// SDK.Install runs CheckTLSConfig before constructing any server, so a removed key stops the
// node with nothing listening; validate runs the same check.
func TestCheckTLSConfigRejectsRemovedViewTimeout(t *testing.T) {
	t.Parallel()
	err := CheckTLSConfig(providerFrom(t, "fsc:\n  web:\n    enabled: true\n    address: 127.0.0.1:0\n    viewTimeout: 10s\n"))
	require.ErrorContains(t, err, "fsc.web.viewtimeout")
	require.ErrorContains(t, err, "fsc.view.remote.timeout")
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
			_, err = OperationsClientAuthWarning(p)
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
		{name: "plaintext web listener", yaml: webOn, want: true, wantWarn: "defaults to true, but the fsc.web.tls listener never requests a client certificate, so /logspec will reject"},
		{name: "plaintext web listener, prometheus", yaml: webOn + "  metrics:\n    provider: prometheus\n", want: true, wantWarn: "so /metrics and /logspec will reject"},
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

	t.Run("enabled without address returns error", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  grpc:
    enabled: true
`)
		srv, err := NewGRPCServer(p)
		require.EqualError(t, err, "fsc.grpc.enabled is true but fsc.grpc.address is not set")
		require.Nil(t, srv)
	})

	t.Run("enabled with malformed address returns error", func(t *testing.T) {
		t.Parallel()
		for _, addr := range []string{"127.0.0.1", "20000"} {
			p := providerFrom(t, "fsc:\n  grpc:\n    enabled: true\n    address: "+addr+"\n")
			srv, err := NewGRPCServer(p)
			require.ErrorContains(t, err, "invalid fsc.grpc.address ["+addr+"]")
			require.Nil(t, srv)
		}
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

	t.Run("with the default time window", func(t *testing.T) {
		t.Parallel()
		srv, err := NewViewServiceServer(RemoteConfig{TimeWindow: DefaultCommandTimeWindow}, nil, nil, nil, tp.Default)
		require.NoError(t, err)
		require.NotNil(t, srv)
	})
}
