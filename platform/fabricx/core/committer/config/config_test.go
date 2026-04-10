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

func TestNewNotificationServiceConfig(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		fakeConfigService := &mock.ServiceBackend{}
		fakeConfigService.UnmarshalKeyStub = func(key string, rawVal interface{}) error {
			if key == "notificationService" {
				if cfg, ok := rawVal.(**config.Config); ok {
					(*cfg).Endpoints = []config.Endpoint{{Address: "test-address"}}
					(*cfg).RequestTimeout = 10 * time.Second
				}
			}
			return nil
		}

		cfg, err := config.NewNotificationServiceConfig(fakeConfigService)
		require.NoError(t, err)
		require.NotNil(t, cfg)
		require.Len(t, cfg.Endpoints, 1)
		require.Equal(t, "test-address", cfg.Endpoints[0].Address)
		require.Equal(t, 10*time.Second, cfg.RequestTimeout)
	})

	t.Run("default timeout", func(t *testing.T) {
		t.Parallel()
		fakeConfigService := &mock.ServiceBackend{}
		fakeConfigService.UnmarshalKeyReturns(nil)

		cfg, err := config.NewNotificationServiceConfig(fakeConfigService)
		require.NoError(t, err)
		require.NotNil(t, cfg)
		require.Equal(t, config.DefaultRequestTimeout, cfg.RequestTimeout)
	})

	t.Run("error unmarshal", func(t *testing.T) {
		t.Parallel()
		fakeConfigService := &mock.ServiceBackend{}
		fakeConfigService.UnmarshalKeyReturns(errors.New("unmarshal-error"))

		cfg, err := config.NewNotificationServiceConfig(fakeConfigService)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unmarshal-error")
		require.NotNil(t, cfg)
	})
}

func TestNewQueryServiceConfig(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		fakeConfigService := &mock.ServiceBackend{}
		fakeConfigService.UnmarshalKeyStub = func(key string, rawVal interface{}) error {
			if key == "queryService" {
				if cfg, ok := rawVal.(**config.Config); ok {
					(*cfg).Endpoints = []config.Endpoint{{Address: "test-address"}}
					(*cfg).RequestTimeout = 10 * time.Second
				}
			}
			return nil
		}

		cfg, err := config.NewQueryServiceConfig(fakeConfigService)
		require.NoError(t, err)
		require.NotNil(t, cfg)
		require.Len(t, cfg.Endpoints, 1)
		require.Equal(t, "test-address", cfg.Endpoints[0].Address)
		require.Equal(t, 10*time.Second, cfg.RequestTimeout)
	})

	t.Run("default timeout", func(t *testing.T) {
		t.Parallel()
		fakeConfigService := &mock.ServiceBackend{}
		fakeConfigService.UnmarshalKeyReturns(nil)

		cfg, err := config.NewQueryServiceConfig(fakeConfigService)
		require.NoError(t, err)
		require.NotNil(t, cfg)
		require.Equal(t, config.DefaultRequestTimeout, cfg.RequestTimeout)
	})

	t.Run("error unmarshal", func(t *testing.T) {
		t.Parallel()
		fakeConfigService := &mock.ServiceBackend{}
		fakeConfigService.UnmarshalKeyReturns(errors.New("unmarshal-error"))

		cfg, err := config.NewQueryServiceConfig(fakeConfigService)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unmarshal-error")
		require.NotNil(t, cfg)
	})
}
