/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config/mock"
)

func TestNewNotificationServiceConfig(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		fakeConfigService := &mock.ServiceBackend{}
		fakeConfigService.UnmarshalKeyStub = func(key string, rawVal any) error {
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

	t.Run("finality defaults when unset", func(t *testing.T) {
		t.Parallel()
		fakeConfigService := &mock.ServiceBackend{}
		fakeConfigService.UnmarshalKeyReturns(nil)

		cfg, err := config.NewNotificationServiceConfig(fakeConfigService)
		require.NoError(t, err)
		require.Equal(t, config.DefaultHandlerTimeout, cfg.HandlerTimeout)
		require.Equal(t, config.DefaultHandlerWorkers, cfg.HandlerWorkers)
		require.Equal(t, config.DefaultListenerTTL, cfg.ListenerTTL)
		require.Equal(t, config.DefaultSweepInterval, cfg.SweepInterval)
	})

	t.Run("explicit zero listenerTTL overrides default to disable expiry", func(t *testing.T) {
		t.Parallel()
		fakeConfigService := &mock.ServiceBackend{}
		fakeConfigService.UnmarshalKeyStub = func(key string, rawVal any) error {
			if cfg, ok := rawVal.(**config.Config); ok {
				(*cfg).ListenerTTL = 0
			}
			return nil
		}

		cfg, err := config.NewNotificationServiceConfig(fakeConfigService)
		require.NoError(t, err)
		require.Zero(t, cfg.ListenerTTL, "an explicit zero must override the default, not be treated as unset")
	})

	t.Run("explicit zero handler and interval settings fall back to defaults", func(t *testing.T) {
		t.Parallel()
		fakeConfigService := &mock.ServiceBackend{}
		fakeConfigService.UnmarshalKeyStub = func(key string, rawVal any) error {
			if cfg, ok := rawVal.(**config.Config); ok {
				(*cfg).HandlerTimeout = 0
				(*cfg).HandlerWorkers = 0
				(*cfg).SweepInterval = 0
			}
			return nil
		}

		cfg, err := config.NewNotificationServiceConfig(fakeConfigService)
		require.NoError(t, err)
		require.Equal(t, config.DefaultHandlerTimeout, cfg.HandlerTimeout, "unlike ListenerTTL, zero has no special meaning here")
		require.Equal(t, config.DefaultSweepInterval, cfg.SweepInterval, "unlike ListenerTTL, zero has no special meaning here")
		// A zero limit would make every errgroup TryGo fail, so no callback would
		// ever run: that cannot be a usable "disabled".
		require.Equal(t, config.DefaultHandlerWorkers, cfg.HandlerWorkers, "a zero handler limit would deliver nothing")
	})

	t.Run("negative handlerWorkers falls back to default", func(t *testing.T) {
		t.Parallel()
		fakeConfigService := &mock.ServiceBackend{}
		fakeConfigService.UnmarshalKeyStub = func(key string, rawVal any) error {
			if cfg, ok := rawVal.(**config.Config); ok {
				(*cfg).HandlerWorkers = -1
			}
			return nil
		}

		cfg, err := config.NewNotificationServiceConfig(fakeConfigService)
		require.NoError(t, err)
		// errgroup.SetLimit panics on a negative limit, so this must be sanitized.
		require.Equal(t, config.DefaultHandlerWorkers, cfg.HandlerWorkers)
	})

	t.Run("configured finality durations are preserved", func(t *testing.T) {
		t.Parallel()
		fakeConfigService := &mock.ServiceBackend{}
		fakeConfigService.UnmarshalKeyStub = func(key string, rawVal any) error {
			if cfg, ok := rawVal.(**config.Config); ok {
				(*cfg).HandlerTimeout = 7 * time.Second
				(*cfg).HandlerWorkers = 4
				(*cfg).ListenerTTL = 3 * time.Minute
				(*cfg).SweepInterval = 45 * time.Second
			}
			return nil
		}

		cfg, err := config.NewNotificationServiceConfig(fakeConfigService)
		require.NoError(t, err)
		require.Equal(t, 7*time.Second, cfg.HandlerTimeout)
		require.Equal(t, 4, cfg.HandlerWorkers)
		require.Equal(t, 3*time.Minute, cfg.ListenerTTL)
		require.Equal(t, 45*time.Second, cfg.SweepInterval)
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

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultConfig()
	require.Equal(t, config.DefaultRequestTimeout, cfg.RequestTimeout)
	require.Equal(t, config.DefaultHandlerTimeout, cfg.HandlerTimeout)
	require.Equal(t, config.DefaultHandlerWorkers, cfg.HandlerWorkers)
	require.Equal(t, config.DefaultListenerTTL, cfg.ListenerTTL)
	require.Equal(t, config.DefaultSweepInterval, cfg.SweepInterval)
}

func TestNewQueryServiceConfig(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		fakeConfigService := &mock.ServiceBackend{}
		fakeConfigService.UnmarshalKeyStub = func(key string, rawVal any) error {
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
