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
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/grpc/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
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
		defer cc.Close()

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
		defer cc.Close()

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
		defer cc.Close()

		invokeHealthCheck(t, cc)
	})
}

// startTestServer starts an in-process gRPC server with a health service on a random
// free port and returns its address. The server is stopped automatically when the test ends.
func startTestServer(t *testing.T, opts ...grpc.ServerOption) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

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
