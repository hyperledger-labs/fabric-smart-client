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

	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/grpc"
	mock2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/grpc/mock"
	"github.com/stretchr/testify/require"
)

func TestClientProvider_NotificationServiceClient(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fakeConfigProvider := &mock2.ConfigProvider{}
		fakeConfigService := &mock2.ConfigServiceGeneric{}
		fakeConfigProvider.GetConfigReturns(fakeConfigService, nil)

		fakeConfigService.UnmarshalKeyCalls(func(key string, rawVal interface{}) error {
			if key == "notificationService" {
				if cfg, ok := rawVal.(**grpc2.Config); ok {
					(*cfg).Endpoints = []grpc2.Endpoint{{Address: "localhost:1234"}}
				}
			}
			return nil
		})

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc, err := cp.NotificationServiceClient("test-network")
		require.NoError(t, err)
		require.NotNil(t, cc)
		require.Equal(t, "localhost:1234", cc.Target())
	})

	t.Run("config provider error", func(t *testing.T) {
		fakeConfigProvider := &mock2.ConfigProvider{}
		fakeConfigProvider.GetConfigReturns(nil, errors.New("config-error"))

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc, err := cp.NotificationServiceClient("test-network")
		require.Error(t, err)
		require.Nil(t, cc)
		require.Contains(t, err.Error(), "config-error")
	})

	t.Run("new config error", func(t *testing.T) {
		fakeConfigProvider := &mock2.ConfigProvider{}
		fakeConfigService := &mock2.ConfigServiceGeneric{}
		fakeConfigProvider.GetConfigReturns(fakeConfigService, nil)
		fakeConfigService.UnmarshalKeyReturns(errors.New("unmarshal-error"))

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc, err := cp.NotificationServiceClient("test-network")
		require.Error(t, err)
		require.Nil(t, cc)
		require.Contains(t, err.Error(), "unmarshal-error")
	})

	t.Run("client conn error (multiple endpoints)", func(t *testing.T) {
		fakeConfigProvider := &mock2.ConfigProvider{}
		fakeConfigService := &mock2.ConfigServiceGeneric{}
		fakeConfigProvider.GetConfigReturns(fakeConfigService, nil)

		fakeConfigService.UnmarshalKeyCalls(func(key string, rawVal interface{}) error {
			if key == "notificationService" {
				if cfg, ok := rawVal.(**grpc2.Config); ok {
					(*cfg).Endpoints = []grpc2.Endpoint{{Address: "localhost:1234"}, {Address: "localhost:5678"}}
				}
			}
			return nil
		})

		cp := grpc2.NewClientProvider(fakeConfigProvider)
		cc, err := cp.NotificationServiceClient("test-network")
		require.Error(t, err)
		require.Nil(t, cc)
		require.Contains(t, err.Error(), "we need a single endpoint")
	})
}

func TestClientConn(t *testing.T) {
	t.Run("no endpoints", func(t *testing.T) {
		cfg := &grpc2.Config{Endpoints: []grpc2.Endpoint{}}
		cc, err := grpc2.ClientConn(cfg)
		require.Error(t, err)
		require.Nil(t, cc)
		require.Contains(t, err.Error(), "we need a single endpoint")
	})

	t.Run("empty address", func(t *testing.T) {
		cfg := &grpc2.Config{Endpoints: []grpc2.Endpoint{{Address: ""}}}
		cc, err := grpc2.ClientConn(cfg)
		require.Error(t, err)
		require.Nil(t, cc)
		require.Equal(t, grpc2.ErrInvalidAddress, err)
	})

	t.Run("success", func(t *testing.T) {
		cfg := &grpc2.Config{Endpoints: []grpc2.Endpoint{{Address: "localhost:1234"}}}
		cc, err := grpc2.ClientConn(cfg)
		require.NoError(t, err)
		require.NotNil(t, cc)
		require.Equal(t, "localhost:1234", cc.Target())
	})
}

func TestWithTLS(t *testing.T) {
	t.Run("tls disabled", func(t *testing.T) {
		endpoint := grpc2.Endpoint{TLSEnabled: false}
		opt := grpc2.WithTLS(endpoint)
		require.NotNil(t, opt)
	})

	t.Run("tls enabled root cert not found", func(t *testing.T) {
		endpoint := grpc2.Endpoint{
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
		err := os.WriteFile(certFile, []byte("invalid-cert"), 0644)
		require.NoError(t, err)

		endpoint := grpc2.Endpoint{
			TLSEnabled:      true,
			TLSRootCertFile: certFile,
		}
		require.Panics(t, func() {
			grpc2.WithTLS(endpoint)
		})
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
