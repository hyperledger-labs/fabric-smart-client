/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/grpc/mock"
)

func TestClientProvider_NotificationServiceClient(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.NotificationServiceConfigStub = func(network string) (*config.Config, error) {
			return &config.Config{
				Endpoints: []config.Endpoint{{Address: "localhost:1234"}},
			}, nil
		}

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc, err := cp.NotificationServiceClient("test-network")
		require.NoError(t, err)
		require.NotNil(t, cc)
		require.Equal(t, "localhost:1234", cc.Target())
	})

	t.Run("config provider error", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.NotificationServiceConfigReturns(nil, errors.New("config-error"))

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc, err := cp.NotificationServiceClient("test-network")
		require.Error(t, err)
		require.Nil(t, cc)
		require.Contains(t, err.Error(), "config-error")
	})

	t.Run("new config error", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.NotificationServiceConfigReturns(nil, errors.New("unmarshal-error"))

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc, err := cp.NotificationServiceClient("test-network")
		require.Error(t, err)
		require.Nil(t, cc)
		require.Contains(t, err.Error(), "unmarshal-error")
	})

	t.Run("client conn error (multiple endpoints)", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.NotificationServiceConfigStub = func(network string) (*config.Config, error) {
			return &config.Config{
				Endpoints: []config.Endpoint{{Address: "localhost:1234"}, {Address: "localhost:5678"}},
			}, nil
		}

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc, err := cp.NotificationServiceClient("test-network")
		require.Error(t, err)
		require.Nil(t, cc)
		require.Contains(t, err.Error(), "we need a single endpoint")
	})

	t.Run("caches per network", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.NotificationServiceConfigStub = func(network string) (*config.Config, error) {
			return &config.Config{
				Endpoints: []config.Endpoint{{Address: "localhost:1234"}},
			}, nil
		}

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc1, err := cp.NotificationServiceClient("net-a")
		require.NoError(t, err)
		cc2, err := cp.NotificationServiceClient("net-a")
		require.NoError(t, err)
		require.Same(t, cc1, cc2)
		require.Equal(t, 1, fakeConfigProvider.NotificationServiceConfigCallCount())

		cc3, err := cp.NotificationServiceClient("net-b")
		require.NoError(t, err)
		require.NotSame(t, cc1, cc3)
		require.Equal(t, 2, fakeConfigProvider.NotificationServiceConfigCallCount())
	})
}

func TestClientProvider_QueryServiceClient(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.QueryServiceConfigStub = func(network string) (*config.Config, error) {
			return &config.Config{
				Endpoints: []config.Endpoint{{Address: "localhost:5678"}},
			}, nil
		}

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc, err := cp.QueryServiceClient("test-network")
		require.NoError(t, err)
		require.NotNil(t, cc)
		require.Equal(t, "localhost:5678", cc.Target())
	})

	t.Run("config provider error", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.QueryServiceConfigReturns(nil, errors.New("config-error"))

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc, err := cp.QueryServiceClient("test-network")
		require.Error(t, err)
		require.Nil(t, cc)
		require.Contains(t, err.Error(), "config-error")
	})

	t.Run("client conn error (multiple endpoints)", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.QueryServiceConfigStub = func(network string) (*config.Config, error) {
			return &config.Config{
				Endpoints: []config.Endpoint{{Address: "localhost:5678"}, {Address: "localhost:9999"}},
			}, nil
		}

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc, err := cp.QueryServiceClient("test-network")
		require.Error(t, err)
		require.Nil(t, cc)
		require.Contains(t, err.Error(), "we need a single endpoint")
	})

	t.Run("caches per network", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.QueryServiceConfigStub = func(network string) (*config.Config, error) {
			return &config.Config{
				Endpoints: []config.Endpoint{{Address: "localhost:5678"}},
			}, nil
		}

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc1, err := cp.QueryServiceClient("net-a")
		require.NoError(t, err)
		cc2, err := cp.QueryServiceClient("net-a")
		require.NoError(t, err)
		require.Same(t, cc1, cc2)
		require.Equal(t, 1, fakeConfigProvider.QueryServiceConfigCallCount())

		cc3, err := cp.QueryServiceClient("net-b")
		require.NoError(t, err)
		require.NotSame(t, cc1, cc3)
		require.Equal(t, 2, fakeConfigProvider.QueryServiceConfigCallCount())
	})

	t.Run("notification and query caches are independent", func(t *testing.T) {
		t.Parallel()
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.NotificationServiceConfigStub = func(network string) (*config.Config, error) {
			return &config.Config{
				Endpoints: []config.Endpoint{{Address: "localhost:1111"}},
			}, nil
		}
		fakeConfigProvider.QueryServiceConfigStub = func(network string) (*config.Config, error) {
			return &config.Config{
				Endpoints: []config.Endpoint{{Address: "localhost:2222"}},
			}, nil
		}

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		notif, err := cp.NotificationServiceClient("net-a")
		require.NoError(t, err)
		query, err := cp.QueryServiceClient("net-a")
		require.NoError(t, err)
		require.NotSame(t, notif, query)
		require.Equal(t, "localhost:1111", notif.Target())
		require.Equal(t, "localhost:2222", query.Target())
	})
}

func TestClientConn(t *testing.T) {
	t.Parallel()
	t.Run("no endpoints", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{Endpoints: []config.Endpoint{}}
		cc, err := grpc2.ClientConn(cfg)
		require.Error(t, err)
		require.Nil(t, cc)
		require.Contains(t, err.Error(), "we need a single endpoint")
	})

	t.Run("empty address", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{Endpoints: []config.Endpoint{{Address: ""}}}
		cc, err := grpc2.ClientConn(cfg)
		require.Error(t, err)
		require.Nil(t, cc)
		require.Equal(t, grpc2.ErrInvalidAddress, err)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{Endpoints: []config.Endpoint{{Address: "localhost:1234"}}}
		cc, err := grpc2.ClientConn(cfg)
		require.NoError(t, err)
		require.NotNil(t, cc)
		require.Equal(t, "localhost:1234", cc.Target())
	})
}

func TestTransportCredentials(t *testing.T) {
	t.Parallel()
	t.Run("nil tls config", func(t *testing.T) {
		t.Parallel()
		creds, err := grpc2.TransportCredentials(nil)
		require.NoError(t, err)
		require.Equal(t, "insecure", creds.Info().SecurityProtocol)
	})

	t.Run("tls disabled", func(t *testing.T) {
		t.Parallel()
		creds, err := grpc2.TransportCredentials(&config.TLSConfig{Enabled: false})
		require.NoError(t, err)
		require.Equal(t, "insecure", creds.Info().SecurityProtocol)
	})

	t.Run("tls enabled no root certs uses system pool", func(t *testing.T) {
		t.Parallel()
		creds, err := grpc2.TransportCredentials(&config.TLSConfig{
			Enabled:       true,
			RootCertPaths: []string{},
		})
		require.NoError(t, err)
		require.Equal(t, "tls", creds.Info().SecurityProtocol)
	})

	t.Run("tls enabled root cert not found", func(t *testing.T) {
		t.Parallel()
		creds, err := grpc2.TransportCredentials(&config.TLSConfig{
			Enabled:       true,
			RootCertPaths: []string{"non-existent-file"},
		})
		require.Error(t, err)
		require.Nil(t, creds)
	})

	t.Run("tls enabled root cert invalid pem", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		certFile := filepath.Join(tmpDir, "cert.pem")
		require.NoError(t, os.WriteFile(certFile, []byte("invalid-cert"), 0o644))

		creds, err := grpc2.TransportCredentials(&config.TLSConfig{
			Enabled:       true,
			RootCertPaths: []string{certFile},
		})
		require.Error(t, err)
		require.Nil(t, creds)
	})

	t.Run("tls enabled with server name override", func(t *testing.T) {
		t.Parallel()
		certFile, _ := generateCertAndKey(t)

		creds, err := grpc2.TransportCredentials(&config.TLSConfig{
			Enabled:            true,
			RootCertPaths:      []string{certFile},
			ServerNameOverride: "test-server",
		})
		require.NoError(t, err)
		require.Equal(t, "tls", creds.Info().SecurityProtocol)
	})

	t.Run("mtls partial config - only client cert", func(t *testing.T) {
		t.Parallel()
		certFile, _ := generateCertAndKey(t)

		creds, err := grpc2.TransportCredentials(&config.TLSConfig{
			Enabled:        true,
			RootCertPaths:  []string{certFile},
			ClientCertPath: certFile,
		})
		require.NoError(t, err)
		require.Equal(t, "tls", creds.Info().SecurityProtocol)
	})

	t.Run("mtls partial config - only client key", func(t *testing.T) {
		t.Parallel()
		certFile, keyFile := generateCertAndKey(t)

		creds, err := grpc2.TransportCredentials(&config.TLSConfig{
			Enabled:       true,
			RootCertPaths: []string{certFile},
			ClientKeyPath: keyFile,
		})
		require.NoError(t, err)
		require.Equal(t, "tls", creds.Info().SecurityProtocol)
	})

	t.Run("mtls mismatched cert and key", func(t *testing.T) {
		t.Parallel()
		certFile1, _ := generateCertAndKey(t)
		_, keyFile2 := generateCertAndKey(t)

		creds, err := grpc2.TransportCredentials(&config.TLSConfig{
			Enabled:        true,
			RootCertPaths:  []string{certFile1},
			ClientCertPath: certFile1,
			ClientKeyPath:  keyFile2,
		})
		require.Error(t, err)
		require.Nil(t, creds)
	})

	t.Run("mtls success", func(t *testing.T) {
		t.Parallel()
		certFile, keyFile := generateCertAndKey(t)

		creds, err := grpc2.TransportCredentials(&config.TLSConfig{
			Enabled:        true,
			RootCertPaths:  []string{certFile},
			ClientCertPath: certFile,
			ClientKeyPath:  keyFile,
		})
		require.NoError(t, err)
		require.Equal(t, "tls", creds.Info().SecurityProtocol)
	})
}

func TestWithConnectionTime(t *testing.T) {
	t.Parallel()
	t.Run("default timeout", func(t *testing.T) {
		t.Parallel()
		opt := grpc2.WithConnectionTime(0)
		require.NotNil(t, opt)
	})

	t.Run("custom timeout", func(t *testing.T) {
		t.Parallel()
		opt := grpc2.WithConnectionTime(10 * time.Second)
		require.NotNil(t, opt)
	})
}

// TestClientConn_Integration tests real gRPC connections with no TLS, server TLS, and mTLS.
// Each sub-test spins up an in-process gRPC server and performs a real health check RPC
// to verify that the TLS handshake completes end-to-end.
func TestClientConn_Integration(t *testing.T) {
	t.Parallel()
	t.Run("no tls", func(t *testing.T) {
		t.Parallel()
		addr := startTestServer(t)

		cfg := &config.Config{
			Endpoints: []config.Endpoint{{Address: addr}},
		}
		cc, err := grpc2.ClientConn(cfg)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = cc.Close()
		})

		invokeHealthCheck(t, cc)
	})

	t.Run("server tls", func(t *testing.T) {
		t.Parallel()
		certFile, keyFile := generateServerCertAndKey(t)

		serverCert, err := tls.LoadX509KeyPair(certFile, keyFile)
		require.NoError(t, err)
		serverTLSCfg := &tls.Config{
			Certificates: []tls.Certificate{serverCert},
			MinVersion:   tls.VersionTLS12,
		}
		addr := startTestServer(t, grpc.Creds(credentials.NewTLS(serverTLSCfg)))

		cfg := &config.Config{
			Endpoints: []config.Endpoint{{
				Address: addr,
				TLS: &config.TLSConfig{
					Enabled:       true,
					RootCertPaths: []string{certFile},
				},
			}},
		}
		cc, err := grpc2.ClientConn(cfg)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = cc.Close()
		})

		invokeHealthCheck(t, cc)
	})

	t.Run("mtls", func(t *testing.T) {
		t.Parallel()
		serverCertFile, serverKeyFile := generateServerCertAndKey(t)
		clientCertFile, clientKeyFile := generateServerCertAndKey(t)

		serverCert, err := tls.LoadX509KeyPair(serverCertFile, serverKeyFile)
		require.NoError(t, err)

		clientCACert, err := os.ReadFile(clientCertFile)
		require.NoError(t, err)
		clientCAs := x509.NewCertPool()
		require.True(t, clientCAs.AppendCertsFromPEM(clientCACert))

		serverTLSCfg := &tls.Config{
			Certificates: []tls.Certificate{serverCert},
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    clientCAs,
			MinVersion:   tls.VersionTLS12,
		}
		addr := startTestServer(t, grpc.Creds(credentials.NewTLS(serverTLSCfg)))

		cfg := &config.Config{
			Endpoints: []config.Endpoint{{
				Address: addr,
				TLS: &config.TLSConfig{
					Enabled:        true,
					RootCertPaths:  []string{serverCertFile},
					ClientCertPath: clientCertFile,
					ClientKeyPath:  clientKeyFile,
				},
			}},
		}
		cc, err := grpc2.ClientConn(cfg)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = cc.Close()
		})

		invokeHealthCheck(t, cc)
	})
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

// generateServerCertAndKey creates a self-signed certificate with IP SAN 127.0.0.1
// and both server and client auth extended key usages, making it suitable for use
// as both a server certificate and an mTLS client certificate in tests.
func generateServerCertAndKey(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	tmpDir := t.TempDir()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"Test"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certFile = filepath.Join(tmpDir, "cert.pem")
	f, err := os.Create(certFile)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	require.NoError(t, f.Close())

	keyDER, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)
	keyFile = filepath.Join(tmpDir, "key.pem")
	f, err = os.Create(keyFile)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(f, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	require.NoError(t, f.Close())

	return certFile, keyFile
}

// generateCertAndKey creates a temporary self-signed certificate and private key,
// writes them to PEM files, and returns their paths.
func generateCertAndKey(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	tmpDir := t.TempDir()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"Test"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certFile = filepath.Join(tmpDir, "cert.pem")
	f, err := os.Create(certFile)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	require.NoError(t, f.Close())

	keyDER, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)
	keyFile = filepath.Join(tmpDir, "key.pem")
	f, err = os.Create(keyFile)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(f, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	require.NoError(t, f.Close())

	return certFile, keyFile
}

// TestClientProvider_CachingPreventsPerCallGoroutineLeak reproduces, in miniature,
// the leak that motivated per-(service,network) connection caching. Every live
// *grpc.ClientConn carries a handful of long-lived worker goroutines (http2
// reader, loopyWriter, dns resolver watcher, callback serializer), so dialing a
// fresh connection on every call — as the provider did before this fix — grows
// the goroutine count linearly and never reclaims the connections.
//
// Both paths run against the same real server with identical usage:
//   - cached:   ClientProvider.QueryServiceClient reuses one connection
//   - uncached: grpc.ClientConn dialed per call and kept alive (the pre-fix
//     behavior: the old QueryServiceClient called ClientConn(cfg) every time and
//     returned it; callers never closed it)
//
// We assert the cached path adds ~no goroutines across many calls while the
// uncached path leaks roughly a connection's worth per call. Assertions use
// NumGoroutine deltas (robust across grpc versions); the per-worker breakdown is
// logged only, since those internal symbol names are version-specific.
//
// This test does NOT call t.Parallel(): runtime.NumGoroutine() is process-global,
// and Go runs non-parallel tests while every t.Parallel() test is paused, which
// gives a clean measurement window.
func TestClientProvider_CachingPreventsPerCallGoroutineLeak(t *testing.T) { //nolint:paralleltest
	addr := startTestServer(t)
	cfg := &config.Config{Endpoints: []config.Endpoint{{Address: addr}}}

	fakeConfigProvider := &mock.ServiceConfigProvider{}
	fakeConfigProvider.QueryServiceConfigStub = func(string) (*config.Config, error) {
		return &config.Config{Endpoints: []config.Endpoint{{Address: addr}}}, nil
	}

	const calls = 30

	// --- cached path (this fix): one shared connection serves every call ---
	cp := grpc2.NewClientProvider(fakeConfigProvider)
	first, err := cp.QueryServiceClient("net")
	require.NoError(t, err)
	invokeHealthCheck(t, first) // establish the single connection up front

	cachedBase := settledGoroutineCount()
	for i := 0; i < calls; i++ {
		cc, err := cp.QueryServiceClient("net")
		require.NoError(t, err)
		require.Same(t, first, cc) // every call returns the cached connection
		invokeHealthCheck(t, cc)
	}
	cachedGrowth := settledGoroutineCount() - cachedBase
	require.Equal(t, 1, fakeConfigProvider.QueryServiceConfigCallCount()) // dialed exactly once

	// --- uncached path (pre-fix behavior): a fresh conn per call, never closed ---
	leaked := make([]*grpc.ClientConn, 0, calls)
	t.Cleanup(func() {
		for _, cc := range leaked {
			_ = cc.Close()
		}
	})
	uncachedBase := settledGoroutineCount()
	for i := 0; i < calls; i++ {
		cc, err := grpc2.ClientConn(cfg) // exactly what QueryServiceClient did before caching
		require.NoError(t, err)
		invokeHealthCheck(t, cc)
		leaked = append(leaked, cc)
	}
	uncachedGrowth := settledGoroutineCount() - uncachedBase

	t.Logf("goroutine growth over %d calls: cached=%d uncached=%d", calls, cachedGrowth, uncachedGrowth)
	t.Logf("per-connection worker goroutines now live: %v", grpcConnGoroutineBreakdown())

	// cached: well under one new goroutine per call (true value ~0).
	require.Less(t, cachedGrowth, calls,
		"cached provider should not grow goroutines per call (grew %d over %d calls)", cachedGrowth, calls)
	// uncached: at least one extra goroutine per call beyond the cached path.
	require.Greater(t, uncachedGrowth, cachedGrowth+calls,
		"dialing per call should leak ~a connection's worth of goroutines each time (cached grew %d, uncached grew %d over %d calls)",
		cachedGrowth, uncachedGrowth, calls)
}

// settledGoroutineCount returns runtime.NumGoroutine() after letting transient
// goroutines (e.g. just-finished RPCs) wind down, so two readings are comparable.
func settledGoroutineCount() int {
	prev := -1
	n := 0
	for i := 0; i < 25; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
		n = runtime.NumGoroutine()
		if n == prev {
			break
		}
		prev = n
	}
	return n
}

// grpcConnGoroutineBreakdown counts live goroutines whose stacks match grpc's
// per-connection workers — the ones the PR's production profile saw accumulate.
// Diagnostic only (logged, never asserted): these internal symbols are grpc
// version-specific, whereas the test's assertions use NumGoroutine deltas.
func grpcConnGoroutineBreakdown() map[string]int {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	dump := string(buf)
	markers := map[string]string{
		"http2.reader":        "http2Client).reader",
		"loopyWriter.run":     "loopyWriter).run",
		"dnsResolver.watcher": "dnsResolver).watcher",
		"callbackSerializer":  "CallbackSerializer).run",
	}
	out := make(map[string]int, len(markers))
	for label, sig := range markers {
		out[label] = strings.Count(dump, sig)
	}
	return out
}
