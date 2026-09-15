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
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

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
		cc, err := cp.QueryServiceClient("test-network")
		require.NoError(t, err)
		require.NotNil(t, cc)
		require.Equal(t, addr, cc.Target())
	})

	t.Run("config provider error", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.QueryServiceConfigReturns(nil, errors.New("boom"))

		cp := config.NewClientProvider(fakeConfigProvider)
		cc, err := cp.QueryServiceClient("test-network")
		require.ErrorContains(t, err, "boom")
		require.Nil(t, cc)
	})

	t.Run("not exactly one endpoint", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.QueryServiceConfigStub = func(string) (*config.Config, error) {
			return &config.Config{Endpoints: []commongrpc.ConnectionConfig{
				{Address: "localhost:1234"}, {Address: "localhost:5678"},
			}}, nil
		}

		cp := config.NewClientProvider(fakeConfigProvider)
		_, err := cp.QueryServiceClient("test-network")
		require.ErrorContains(t, err, "exactly one endpoint")
	})

	t.Run("empty address", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.QueryServiceConfigStub = func(string) (*config.Config, error) {
			return &config.Config{Endpoints: []commongrpc.ConnectionConfig{{Address: ""}}}, nil
		}

		cp := config.NewClientProvider(fakeConfigProvider)
		_, err := cp.QueryServiceClient("test-network")
		require.ErrorContains(t, err, "address is empty")
	})
}

func TestClientProvider_NotificationServiceClient(t *testing.T) {
	t.Parallel()

	addr := startTestServer(t)
	fakeConfigProvider := &mock.ServiceConfigProvider{}
	fakeConfigProvider.NotificationServiceConfigStub = func(string) (*config.Config, error) {
		return &config.Config{Endpoints: []commongrpc.ConnectionConfig{{Address: addr}}}, nil
	}

	cp := config.NewClientProvider(fakeConfigProvider)
	cc, err := cp.NotificationServiceClient("test-network")
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
	first, err := cp.QueryServiceClient("net")
	require.NoError(t, err)
	invokeHealthCheck(t, first)

	for range 30 {
		cc, err := cp.QueryServiceClient("net")
		require.NoError(t, err)
		require.Same(t, first, cc)
	}
	require.Equal(t, 1, fakeConfigProvider.QueryServiceConfigCallCount())

	// A different network is a different connection.
	other, err := cp.QueryServiceClient("other-net")
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
	cc, err := cp.QueryServiceClient("net")
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
