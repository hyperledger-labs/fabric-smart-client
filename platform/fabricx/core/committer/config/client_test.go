/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config_test

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	commongrpc "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/tlsgen"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config/mock"
)

func TestClientProvider_QueryServiceClient(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		addr := startTestServer(t)
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.QueryServiceConfigStub = func(string) (*config.Config, error) {
			return &config.Config{Endpoints: []commongrpc.ConnectionConfig{{Address: addr}}}, nil
		}

		cp := config.NewClientProvider(fakeConfigProvider)
		cc, err := cp.QueryServiceClient("test-network", "")
		require.NoError(t, err)
		require.NotNil(t, cc)
		require.Equal(t, addr, cc.Target())
	})

	t.Run("config provider error", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.QueryServiceConfigReturns(nil, errors.New("boom"))

		cp := config.NewClientProvider(fakeConfigProvider)
		cc, err := cp.QueryServiceClient("test-network", "")
		require.ErrorContains(t, err, "boom")
		require.Nil(t, cc)
	})

	t.Run("no endpoints", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.QueryServiceConfigStub = func(string) (*config.Config, error) {
			return &config.Config{Endpoints: []commongrpc.ConnectionConfig{}}, nil
		}

		cp := config.NewClientProvider(fakeConfigProvider)
		_, err := cp.QueryServiceClient("test-network", "")
		require.ErrorContains(t, err, "expected at least one endpoint")
	})

	t.Run("empty address", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.QueryServiceConfigStub = func(string) (*config.Config, error) {
			return &config.Config{Endpoints: []commongrpc.ConnectionConfig{{Address: ""}}}, nil
		}

		cp := config.NewClientProvider(fakeConfigProvider)
		_, err := cp.QueryServiceClient("test-network", "")
		require.ErrorContains(t, err, "address is empty")
	})
}

func TestClientProvider_SkipsFailingEndpoint(t *testing.T) {
	t.Parallel()
	addr := startTestServer(t)
	fakeConfigProvider := &mock.ServiceConfigProvider{}
	fakeConfigProvider.QueryServiceConfigStub = func(string) (*config.Config, error) {
		// Nothing listens on port 1, so the first endpoint is unreachable.
		return &config.Config{Endpoints: []commongrpc.ConnectionConfig{
			{Address: "127.0.0.1:1"}, {Address: addr},
		}}, nil
	}

	cp := config.NewClientProvider(fakeConfigProvider)
	require.Eventually(t, func() bool {
		cc, err := cp.QueryServiceClient("test-network", "")
		return err == nil && cc.Target() == addr
	}, 5*time.Second, 10*time.Millisecond, "once the unreachable endpoint fails, the next one must be handed out")
}

// TestClientProvider_SkipsNamedEndpoint covers a retry right after a call failed: gRPC can
// fail the call before the connection reports TRANSIENT_FAILURE, so the failed endpoint
// still looks healthy and only the skip keeps the retry off it. Every endpoint here is up,
// so none is ever failing and the pick depends on the skip alone, not on timing.
func TestClientProvider_SkipsNamedEndpoint(t *testing.T) {
	t.Parallel()
	a, b := startTestServer(t), startTestServer(t)
	for _, tt := range []struct {
		name      string
		endpoints []string
		skip      string
		want      string
	}{
		{name: "no skip", endpoints: []string{a, b}, skip: "", want: a},
		{name: "skip first", endpoints: []string{a, b}, skip: a, want: b},
		{name: "skip unknown", endpoints: []string{a, b}, skip: "127.0.0.1:1", want: a},
		{name: "skip only endpoint", endpoints: []string{a}, skip: a, want: a},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fakeConfigProvider := &mock.ServiceConfigProvider{}
			fakeConfigProvider.QueryServiceConfigStub = func(string) (*config.Config, error) {
				var endpoints []commongrpc.ConnectionConfig
				for _, addr := range tt.endpoints {
					endpoints = append(endpoints, commongrpc.ConnectionConfig{Address: addr})
				}
				return &config.Config{Endpoints: endpoints}, nil
			}

			cc, err := config.NewClientProvider(fakeConfigProvider).QueryServiceClient("test-network", tt.skip)
			require.NoError(t, err)
			require.Equal(t, tt.want, cc.Target())
		})
	}
}

func TestClientProvider_AllFailingFallsBackToFirst(t *testing.T) {
	t.Parallel()
	fakeConfigProvider := &mock.ServiceConfigProvider{}
	fakeConfigProvider.QueryServiceConfigStub = func(string) (*config.Config, error) {
		// Nothing listens on port 1 and 2, so the first endpoint is unreachable.
		return &config.Config{Endpoints: []commongrpc.ConnectionConfig{
			{Address: "127.0.0.1:1"}, {Address: "127.0.0.1:2"},
		}}, nil
	}

	cp := config.NewClientProvider(fakeConfigProvider)
	require.Eventually(t, func() bool {
		cc, err := cp.QueryServiceClient("test-network", "")
		return err == nil && cc.Target() == "127.0.0.1:1" && cc.GetState() == connectivity.TransientFailure // pick hands out a TRANSIENT_FAILURE conn only when every conn is in it.
	}, 5*time.Second, 10*time.Millisecond, "when every endpoint is failing, the first one must be handed out")
}

func TestClientProvider_NotificationServiceClient(t *testing.T) {
	t.Parallel()

	addr := startTestServer(t)
	fakeConfigProvider := &mock.ServiceConfigProvider{}
	fakeConfigProvider.NotificationServiceConfigStub = func(string) (*config.Config, error) {
		return &config.Config{Endpoints: []commongrpc.ConnectionConfig{{Address: addr}}}, nil
	}

	cp := config.NewClientProvider(fakeConfigProvider)
	cc, err := cp.NotificationServiceClient("test-network", "")
	require.NoError(t, err)
	require.Equal(t, addr, cc.Target())
}

// TestClientProvider_CachesPerNetwork replaces the goroutine-delta leak test the old package
// carried. The leak it guarded against was one fresh, never-closed connection per call; the
// direct assertion is that repeated calls return the SAME connection and dial once, which is
// both what fixes the leak and what lazy.Provider now guarantees.
func TestClientProvider_CachesPerNetwork(t *testing.T) {
	t.Parallel()

	addr := startTestServer(t)
	fakeConfigProvider := &mock.ServiceConfigProvider{}
	fakeConfigProvider.QueryServiceConfigStub = func(string) (*config.Config, error) {
		return &config.Config{Endpoints: []commongrpc.ConnectionConfig{{Address: addr}}}, nil
	}

	cp := config.NewClientProvider(fakeConfigProvider)
	first, err := cp.QueryServiceClient("net", "")
	require.NoError(t, err)
	invokeHealthCheck(t, first)

	for range 30 {
		cc, err := cp.QueryServiceClient("net", "")
		require.NoError(t, err)
		require.Same(t, first, cc)
	}
	require.Equal(t, 1, fakeConfigProvider.QueryServiceConfigCallCount())

	// A different network is a different connection.
	other, err := cp.QueryServiceClient("other-net", "")
	require.NoError(t, err)
	require.NotSame(t, first, other)
	require.Equal(t, 2, fakeConfigProvider.QueryServiceConfigCallCount())
}

// TestClientProvider_TLS13 is the port of the old package's TestClientConn_Integration: the
// TLS 1.3 requirement that used to be hardcoded in fabricx's own TransportCredentials now
// comes from the endpoint's resolved TLS, and must still produce a working handshake against
// a TLS 1.3 server.
func TestClientProvider_TLS13(t *testing.T) {
	t.Parallel()

	authority, err := tlsgen.NewCA()
	require.NoError(t, err)
	kp, err := authority.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)
	addr := startTLSTestServer(t, kp.Cert, kp.Key)

	fakeConfigProvider := &mock.ServiceConfigProvider{}
	fakeConfigProvider.QueryServiceConfigStub = func(string) (*config.Config, error) {
		return &config.Config{Endpoints: []commongrpc.ConnectionConfig{{
			Address: addr,
			TLS: commongrpc.SecureOptions{
				UseTLS:        true,
				ServerRootCAs: [][]byte{authority.CertBytes()},
				MinVersion:    tls.VersionTLS13,
			},
		}}}, nil
	}

	cp := config.NewClientProvider(fakeConfigProvider)
	cc, err := cp.QueryServiceClient("net", "")
	require.NoError(t, err)
	invokeHealthCheck(t, cc)
}

// startTestServer starts an in-process gRPC server with a health service on a random
// free port and returns its address. The server is stopped automatically when the test ends.
func startTestServer(t *testing.T, opts ...grpc.ServerOption) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = lis.Close()
	})

	srv := grpc.NewServer(opts...)
	hs := health.NewServer()
	healthpb.RegisterHealthServer(srv, hs)
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	return lis.Addr().String()
}

// startTLSTestServer is the TLS variant of startTestServer, serving with the given
// PEM-encoded certificate and key and requiring TLS 1.3.
func startTLSTestServer(t *testing.T, certPEM, keyPEM []byte) string {
	t.Helper()

	serverCert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	serverTLSCfg := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		MinVersion:   tls.VersionTLS13,
	}

	return startTestServer(t, grpc.Creds(credentials.NewTLS(serverTLSCfg)))
}

// invokeHealthCheck performs a health check RPC on the given connection, asserting
// that the call succeeds and the server reports SERVING status.
func invokeHealthCheck(t *testing.T, cc *grpc.ClientConn) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := healthpb.NewHealthClient(cc).Check(ctx, &healthpb.HealthCheckRequest{Service: ""})
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.GetStatus())
}

// TestClientProvider_KeepAliveDetectsSilentCut checks that the configured keepalive reaches
// the connection: once the link stops forwarding without closing, the client's ping goes
// unanswered and gRPC drops the connection, while without keepalive it stays READY. gRPC
// raises a client ping interval below 10 s to 10 s, so detection takes about that plus the
// timeout.
func TestClientProvider_KeepAliveDetectsSilentCut(t *testing.T) {
	t.Parallel()
	const pingInterval = 10 * time.Second
	for _, tt := range []struct {
		name      string
		keepAlive *commongrpc.ClientKeepAliveConfig
		dropped   bool
	}{
		{name: "keepalive", keepAlive: &commongrpc.ClientKeepAliveConfig{Time: pingInterval, Timeout: time.Second, PermitWithoutStream: true}, dropped: true},
		{name: "no keepalive", dropped: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			addr := startTestServer(t, grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{MinTime: time.Millisecond, PermitWithoutStream: true}))
			link := newSilentLink(t, addr)
			fakeConfigProvider := &mock.ServiceConfigProvider{}
			fakeConfigProvider.QueryServiceConfigStub = func(string) (*config.Config, error) {
				return &config.Config{Endpoints: []commongrpc.ConnectionConfig{{Address: link.addr()}}, KeepAlive: tt.keepAlive}, nil
			}

			cc, err := config.NewClientProvider(fakeConfigProvider).QueryServiceClient("test-network", "")
			require.NoError(t, err)
			cc.Connect()
			require.Eventually(t, func() bool { return cc.GetState() == connectivity.Ready }, 5*time.Second, 10*time.Millisecond)

			link.cut()
			if tt.dropped {
				require.Eventually(t, func() bool { return cc.GetState() != connectivity.Ready }, 2*pingInterval+5*time.Second, 50*time.Millisecond)
			} else {
				require.Never(t, func() bool { return cc.GetState() != connectivity.Ready }, pingInterval+5*time.Second, 100*time.Millisecond)
			}
		})
	}
}

// silentLink forwards TCP connections to a backend until cut, after which it stops
// forwarding without closing anything, so neither side sees a reset.
type silentLink struct {
	listener net.Listener
	cutCh    chan struct{}
}

func newSilentLink(t *testing.T, backend string) *silentLink {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	link := &silentLink{listener: l, cutCh: make(chan struct{})}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			b, err := net.Dial("tcp", backend)
			if err != nil {
				_ = c.Close()
				continue
			}
			t.Cleanup(func() { _ = c.Close(); _ = b.Close() })
			go link.pipe(b, c)
			go link.pipe(c, b)
		}
	}()
	return link
}

func (l *silentLink) addr() string { return l.listener.Addr().String() }

func (l *silentLink) cut() { close(l.cutCh) }

func (l *silentLink) pipe(dst, src net.Conn) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		select {
		case <-l.cutCh:
			return
		default:
		}
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}
