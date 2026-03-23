/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc_test

import (
	"errors"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/grpc/mock"
	"github.com/stretchr/testify/require"
)

func TestNewNotificationServiceConfig(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fakeConfigService := &mock.ConfigService{}
		fakeConfigService.UnmarshalKeyCalls(func(key string, rawVal interface{}) error {
			if key == "notificationService" {
				if cfg, ok := rawVal.(**grpc.Config); ok {
					(*cfg).Endpoints = []grpc.Endpoint{{Address: "test-address"}}
					(*cfg).RequestTimeout = 10 * time.Second
				}
			}
			return nil
		})

		cfg, err := grpc.NewNotificationServiceConfig(fakeConfigService)
		require.NoError(t, err)
		require.NotNil(t, cfg)
		require.Equal(t, 1, len(cfg.Endpoints))
		require.Equal(t, "test-address", cfg.Endpoints[0].Address)
		require.Equal(t, 10*time.Second, cfg.RequestTimeout)
	})

	t.Run("default timeout", func(t *testing.T) {
		fakeConfigService := &mock.ConfigService{}
		fakeConfigService.UnmarshalKeyReturns(nil)

		cfg, err := grpc.NewNotificationServiceConfig(fakeConfigService)
		require.NoError(t, err)
		require.NotNil(t, cfg)
		require.Equal(t, grpc.DefaultRequestTimeout, cfg.RequestTimeout)
	})

	t.Run("error unmarshal", func(t *testing.T) {
		fakeConfigService := &mock.ConfigService{}
		fakeConfigService.UnmarshalKeyReturns(errors.New("unmarshal-error"))

		cfg, err := grpc.NewNotificationServiceConfig(fakeConfigService)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unmarshal-error")
		require.NotNil(t, cfg)
	})
}
