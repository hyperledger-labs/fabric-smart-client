/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config_test

import (
	"testing"
	"time"

	channelconfig "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channel/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channel/config/mock"
	"github.com/stretchr/testify/require"
)

func TestNewConfig(t *testing.T) {
	t.Parallel()
	t.Run("with default values", func(t *testing.T) {
		t.Parallel()
		cs := &mock.ConfigService{}
		config, err := channelconfig.NewConfig(cs)
		require.NoError(t, err)
		require.NotNil(t, config)
		require.Equal(t, 1*time.Minute, config.PollInterval)
		require.Equal(t, 5, config.MaxRetries)
		require.Equal(t, 1*time.Second, config.InitialRetryDelay)
		require.Equal(t, 5*time.Minute, config.MaxRetryDelay)
	})

	t.Run("with custom values", func(t *testing.T) {
		t.Parallel()
		cs := &mock.ConfigService{}
		cs.IsSetReturnsOnCall(0, true) // pollInterval
		cs.GetDurationReturnsOnCall(0, 30*time.Second)
		cs.IsSetReturnsOnCall(1, true) // maxRetries
		cs.GetIntReturnsOnCall(0, 10)
		cs.IsSetReturnsOnCall(2, true) // initialRetryDelay
		cs.GetDurationReturnsOnCall(1, 2*time.Second)
		cs.IsSetReturnsOnCall(3, true) // maxRetryDelay
		cs.GetDurationReturnsOnCall(2, 10*time.Minute)

		config, err := channelconfig.NewConfig(cs)
		require.NoError(t, err)
		require.NotNil(t, config)
		require.Equal(t, 30*time.Second, config.PollInterval)
		require.Equal(t, 10, config.MaxRetries)
		require.Equal(t, 2*time.Second, config.InitialRetryDelay)
		require.Equal(t, 10*time.Minute, config.MaxRetryDelay)
	})

	t.Run("with all custom values set", func(t *testing.T) {
		t.Parallel()
		cs := &mock.ConfigService{}
		cs.IsSetReturns(true)
		cs.GetDurationReturns(45 * time.Second)

		config, err := channelconfig.NewConfig(cs)
		require.NoError(t, err)
		require.NotNil(t, config)
		require.Equal(t, 45*time.Second, config.PollInterval)
	})
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()
	t.Run("valid configuration", func(t *testing.T) {
		t.Parallel()
		config := &channelconfig.Config{
			PollInterval:      1 * time.Minute,
			MaxRetries:        5,
			InitialRetryDelay: 1 * time.Second,
			MaxRetryDelay:     5 * time.Minute,
		}
		err := config.Validate()
		require.NoError(t, err)
	})

	t.Run("invalid poll interval - zero", func(t *testing.T) {
		t.Parallel()
		config := &channelconfig.Config{
			PollInterval:      0,
			MaxRetries:        5,
			InitialRetryDelay: 1 * time.Second,
			MaxRetryDelay:     5 * time.Minute,
		}
		err := config.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "pollInterval must be positive")
	})

	t.Run("invalid poll interval - negative", func(t *testing.T) {
		t.Parallel()
		config := &channelconfig.Config{
			PollInterval:      -1 * time.Second,
			MaxRetries:        5,
			InitialRetryDelay: 1 * time.Second,
			MaxRetryDelay:     5 * time.Minute,
		}
		err := config.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "pollInterval must be positive")
	})

	t.Run("invalid max retries - negative", func(t *testing.T) {
		t.Parallel()
		config := &channelconfig.Config{
			PollInterval:      1 * time.Minute,
			MaxRetries:        -1,
			InitialRetryDelay: 1 * time.Second,
			MaxRetryDelay:     5 * time.Minute,
		}
		err := config.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "maxRetries must be non-negative")
	})

	t.Run("valid max retries - zero", func(t *testing.T) {
		t.Parallel()
		config := &channelconfig.Config{
			PollInterval:      1 * time.Minute,
			MaxRetries:        0,
			InitialRetryDelay: 1 * time.Second,
			MaxRetryDelay:     5 * time.Minute,
		}
		err := config.Validate()
		require.NoError(t, err)
	})

	t.Run("invalid initial retry delay - zero", func(t *testing.T) {
		t.Parallel()
		config := &channelconfig.Config{
			PollInterval:      1 * time.Minute,
			MaxRetries:        5,
			InitialRetryDelay: 0,
			MaxRetryDelay:     5 * time.Minute,
		}
		err := config.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "initialRetryDelay must be positive")
	})

	t.Run("invalid max retry delay - zero", func(t *testing.T) {
		t.Parallel()
		config := &channelconfig.Config{
			PollInterval:      1 * time.Minute,
			MaxRetries:        5,
			InitialRetryDelay: 1 * time.Second,
			MaxRetryDelay:     0,
		}
		err := config.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "maxRetryDelay must be positive")
	})

	t.Run("invalid - initial delay exceeds max delay", func(t *testing.T) {
		t.Parallel()
		config := &channelconfig.Config{
			PollInterval:      1 * time.Minute,
			MaxRetries:        5,
			InitialRetryDelay: 10 * time.Minute,
			MaxRetryDelay:     5 * time.Minute,
		}
		err := config.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "initialRetryDelay")
		require.Contains(t, err.Error(), "must not exceed maxRetryDelay")
	})
}
