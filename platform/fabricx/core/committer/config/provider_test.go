/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config_test

import (
	"errors"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config/mock"
	"github.com/stretchr/testify/require"
)

func TestConfigProvider_NotificationServiceConfig(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fakeConfigProvider := &mock.FabricConfigProvider{}
		fakeConfigService := &mock.FabricConfigService{}
		fakeConfigProvider.GetConfigReturns(fakeConfigService, nil)

		fakeConfigService.UnmarshalKeyStub = func(key string, rawVal interface{}) error {
			if key == "notificationService" {
				if cfg, ok := rawVal.(**config.Config); ok {
					(*cfg).Endpoints = []config.Endpoint{{Address: "localhost:1234"}}
					(*cfg).RequestTimeout = 10 * time.Second
				}
			}
			return nil
		}

		cp := config.NewProvider(fakeConfigProvider)
		cfg, err := cp.NotificationServiceConfig("test-network")
		require.NoError(t, err)
		require.NotNil(t, cfg)
		require.Equal(t, 1, len(cfg.Endpoints))
		require.Equal(t, "localhost:1234", cfg.Endpoints[0].Address)
		require.Equal(t, 10*time.Second, cfg.RequestTimeout)
	})

	t.Run("config provider error", func(t *testing.T) {
		fakeConfigProvider := &mock.FabricConfigProvider{}
		fakeConfigProvider.GetConfigReturns(nil, errors.New("config-error"))

		cp := config.NewProvider(fakeConfigProvider)
		cfg, err := cp.NotificationServiceConfig("test-network")
		require.Error(t, err)
		require.Nil(t, cfg)
		require.Contains(t, err.Error(), "config-error")
	})

	t.Run("new config error", func(t *testing.T) {
		fakeConfigProvider := &mock.FabricConfigProvider{}
		fakeConfigService := &mock.FabricConfigService{}
		fakeConfigProvider.GetConfigReturns(fakeConfigService, nil)
		fakeConfigService.UnmarshalKeyReturns(errors.New("unmarshal-error"))

		cp := config.NewProvider(fakeConfigProvider)
		cfg, err := cp.NotificationServiceConfig("test-network")
		require.Error(t, err)
		require.Nil(t, cfg)
		require.Contains(t, err.Error(), "unmarshal-error")
	})
}

func TestConfigProvider_QueryServiceConfig(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fakeConfigProvider := &mock.FabricConfigProvider{}
		fakeConfigService := &mock.FabricConfigService{}
		fakeConfigProvider.GetConfigReturns(fakeConfigService, nil)

		fakeConfigService.UnmarshalKeyStub = func(key string, rawVal interface{}) error {
			if key == "queryService" {
				if cfg, ok := rawVal.(**config.Config); ok {
					(*cfg).Endpoints = []config.Endpoint{{Address: "localhost:5678"}}
					(*cfg).RequestTimeout = 15 * time.Second
				}
			}
			return nil
		}

		cp := config.NewProvider(fakeConfigProvider)
		cfg, err := cp.QueryServiceConfig("test-network")
		require.NoError(t, err)
		require.NotNil(t, cfg)
		require.Equal(t, 1, len(cfg.Endpoints))
		require.Equal(t, "localhost:5678", cfg.Endpoints[0].Address)
		require.Equal(t, 15*time.Second, cfg.RequestTimeout)
	})

	t.Run("config provider error", func(t *testing.T) {
		fakeConfigProvider := &mock.FabricConfigProvider{}
		fakeConfigProvider.GetConfigReturns(nil, errors.New("config-error"))

		cp := config.NewProvider(fakeConfigProvider)
		cfg, err := cp.QueryServiceConfig("test-network")
		require.Error(t, err)
		require.Nil(t, cfg)
		require.Contains(t, err.Error(), "config-error")
	})

	t.Run("new config error", func(t *testing.T) {
		fakeConfigProvider := &mock.FabricConfigProvider{}
		fakeConfigService := &mock.FabricConfigService{}
		fakeConfigProvider.GetConfigReturns(fakeConfigService, nil)
		fakeConfigService.UnmarshalKeyReturns(errors.New("unmarshal-error"))

		cp := config.NewProvider(fakeConfigProvider)
		cfg, err := cp.QueryServiceConfig("test-network")
		require.Error(t, err)
		require.Nil(t, cfg)
		require.Contains(t, err.Error(), "unmarshal-error")
	})
}
