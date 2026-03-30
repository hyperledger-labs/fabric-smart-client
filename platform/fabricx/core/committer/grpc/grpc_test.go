/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/grpc/mock"
	"github.com/stretchr/testify/require"
)

func TestClientProvider_NotificationServiceClient(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.NotificationServiceConfigReturns(nil, errors.New("config-error"))

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc, err := cp.NotificationServiceClient("test-network")
		require.Error(t, err)
		require.Nil(t, cc)
		require.Contains(t, err.Error(), "config-error")
	})

	t.Run("new config error", func(t *testing.T) {
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.NotificationServiceConfigReturns(nil, errors.New("unmarshal-error"))

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc, err := cp.NotificationServiceClient("test-network")
		require.Error(t, err)
		require.Nil(t, cc)
		require.Contains(t, err.Error(), "unmarshal-error")
	})

	t.Run("client conn error (multiple endpoints)", func(t *testing.T) {
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
	t.Run("success", func(t *testing.T) {
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
		fakeConfigProvider := &mock.ServiceConfigProvider{}
		fakeConfigProvider.QueryServiceConfigReturns(nil, errors.New("config-error"))

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc, err := cp.QueryServiceClient("test-network")
		require.Error(t, err)
		require.Nil(t, cc)
		require.Contains(t, err.Error(), "config-error")
	})

	t.Run("client conn error (multiple endpoints)", func(t *testing.T) {
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
	t.Run("no endpoints", func(t *testing.T) {
		cfg := &config.Config{Endpoints: []config.Endpoint{}}
		cc, err := grpc2.ClientConn(cfg)
		require.Error(t, err)
		require.Nil(t, cc)
		require.Contains(t, err.Error(), "we need a single endpoint")
	})

	t.Run("empty address", func(t *testing.T) {
		cfg := &config.Config{Endpoints: []config.Endpoint{{Address: ""}}}
		cc, err := grpc2.ClientConn(cfg)
		require.Error(t, err)
		require.Nil(t, cc)
		require.Equal(t, grpc2.ErrInvalidAddress, err)
	})

	t.Run("success", func(t *testing.T) {
		cfg := &config.Config{Endpoints: []config.Endpoint{{Address: "localhost:1234"}}}
		cc, err := grpc2.ClientConn(cfg)
		require.NoError(t, err)
		require.NotNil(t, cc)
		require.Equal(t, "localhost:1234", cc.Target())
	})
}

func TestWithTLS(t *testing.T) {
	t.Run("tls disabled", func(t *testing.T) {
		endpoint := config.Endpoint{TLSEnabled: false}
		opt := grpc2.WithTLS(endpoint)
		require.NotNil(t, opt)
	})

	t.Run("tls enabled root cert not found", func(t *testing.T) {
		endpoint := config.Endpoint{
			TLSEnabled:      true,
			TLSRootCertFile: "non-existent-file",
		}
		require.Panics(t, func() {
			grpc2.WithTLS(endpoint)
		})
	})

	t.Run("tls enabled root cert exists invalid cert", func(t *testing.T) {
		tmpDir := t.TempDir()
		certFile := filepath.Join(tmpDir, "cert.pem")
		err := os.WriteFile(certFile, []byte("invalid-cert"), 0o644)
		require.NoError(t, err)

		endpoint := config.Endpoint{
			TLSEnabled:      true,
			TLSRootCertFile: certFile,
		}
		require.Panics(t, func() {
			grpc2.WithTLS(endpoint)
		})
	})

	t.Run("tls enabled with server name override", func(t *testing.T) {
		tmpDir := t.TempDir()
		certFile := filepath.Join(tmpDir, "cert.pem")
		// Create a valid self-signed certificate for testing
		certPEM := []byte(`-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----`)
		err := os.WriteFile(certFile, certPEM, 0o644)
		require.NoError(t, err)

		endpoint := config.Endpoint{
			TLSEnabled:            true,
			TLSRootCertFile:       certFile,
			TLSServerNameOverride: "test-server",
		}
		opt := grpc2.WithTLS(endpoint)
		require.NotNil(t, opt)
	})
}

func TestWithConnectionTime(t *testing.T) {
	t.Run("default timeout", func(t *testing.T) {
		opt := grpc2.WithConnectionTime(0)
		require.NotNil(t, opt)
	})

	t.Run("custom timeout", func(t *testing.T) {
		opt := grpc2.WithConnectionTime(10 * time.Second)
		require.NotNil(t, opt)
	})
}
