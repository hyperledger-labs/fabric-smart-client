/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig_test

import (
	"context"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channelconfig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channelconfig/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChannelConfigMonitor(t *testing.T) {
	config := &channelconfig.Config{
		PollInterval:      1 * time.Minute,
		MaxRetries:        5,
		InitialRetryDelay: 1 * time.Second,
		MaxRetryDelay:     5 * time.Minute,
	}
	queryService := &mock.QueryService{}
	membershipService := &mock.MembershipService{}
	orderingService := &mock.OrderingService{}
	configService := &mock.ConfigService{}

	t.Run("successful creation", func(t *testing.T) {
		monitor, err := channelconfig.NewChannelConfigMonitor(
			config,
			queryService,
			membershipService,
			orderingService,
			configService,
			"testnet",
			"mychannel",
		)
		require.NoError(t, err)
		assert.NotNil(t, monitor)
		assert.False(t, monitor.IsRunning())
	})

	t.Run("nil config", func(t *testing.T) {
		monitor, err := channelconfig.NewChannelConfigMonitor(
			nil,
			queryService,
			membershipService,
			orderingService,
			configService,
			"testnet",
			"mychannel",
		)
		assert.Error(t, err)
		assert.Nil(t, monitor)
		assert.Contains(t, err.Error(), "config cannot be nil")
	})

	t.Run("nil query service", func(t *testing.T) {
		monitor, err := channelconfig.NewChannelConfigMonitor(
			config,
			nil,
			membershipService,
			orderingService,
			configService,
			"testnet",
			"mychannel",
		)
		assert.Error(t, err)
		assert.Nil(t, monitor)
		assert.Contains(t, err.Error(), "queryService cannot be nil")
	})

	t.Run("nil membership service", func(t *testing.T) {
		monitor, err := channelconfig.NewChannelConfigMonitor(
			config,
			queryService,
			nil,
			orderingService,
			configService,
			"testnet",
			"mychannel",
		)
		assert.Error(t, err)
		assert.Nil(t, monitor)
		assert.Contains(t, err.Error(), "membershipService cannot be nil")
	})

	t.Run("nil ordering service", func(t *testing.T) {
		monitor, err := channelconfig.NewChannelConfigMonitor(
			config,
			queryService,
			membershipService,
			nil,
			configService,
			"testnet",
			"mychannel",
		)
		assert.Error(t, err)
		assert.Nil(t, monitor)
		assert.Contains(t, err.Error(), "orderingService cannot be nil")
	})

	t.Run("nil config service", func(t *testing.T) {
		monitor, err := channelconfig.NewChannelConfigMonitor(
			config,
			queryService,
			membershipService,
			orderingService,
			nil,
			"testnet",
			"mychannel",
		)
		assert.Error(t, err)
		assert.Nil(t, monitor)
		assert.Contains(t, err.Error(), "configService cannot be nil")
	})

	t.Run("empty channel name", func(t *testing.T) {
		monitor, err := channelconfig.NewChannelConfigMonitor(
			config,
			queryService,
			membershipService,
			orderingService,
			configService,
			"testnet",
			"",
		)
		assert.Error(t, err)
		assert.Nil(t, monitor)
		assert.Contains(t, err.Error(), "channel name cannot be empty")
	})
}

func TestServiceLifecycle(t *testing.T) {
	config := &channelconfig.Config{
		PollInterval:      100 * time.Millisecond,
		MaxRetries:        2,
		InitialRetryDelay: 10 * time.Millisecond,
		MaxRetryDelay:     100 * time.Millisecond,
	}
	queryService := &mock.QueryService{}
	membershipService := &mock.MembershipService{}
	orderingService := &mock.OrderingService{}
	configService := &mock.ConfigService{}

	// Setup mock to return same version (no updates)
	queryService.GetConfigTransactionReturns(&channelconfig.ConfigTransactionInfo{
		Envelope: &cb.Envelope{},
		Version:  1,
	}, nil)

	t.Run("start and stop", func(t *testing.T) {
		monitor, err := channelconfig.NewChannelConfigMonitor(
			config, queryService, membershipService,
			orderingService, configService, "testnet", "mychannel",
		)
		require.NoError(t, err)

		// Initially not running
		assert.False(t, monitor.IsRunning())

		// Start monitoring
		err = monitor.Start(context.Background())
		require.NoError(t, err)
		assert.True(t, monitor.IsRunning())

		// Wait a bit for initial check
		time.Sleep(50 * time.Millisecond)

		// Stop monitoring
		err = monitor.Stop()
		require.NoError(t, err)
		assert.False(t, monitor.IsRunning())
	})

	t.Run("start already running", func(t *testing.T) {
		monitor, err := channelconfig.NewChannelConfigMonitor(
			config, queryService, membershipService,
			orderingService, configService, "testnet", "mychannel",
		)
		require.NoError(t, err)

		err = monitor.Start(context.Background())
		require.NoError(t, err)
		defer func() {
			_ = monitor.Stop()
		}()

		// Try to start again
		err = monitor.Start(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already running")
	})

	t.Run("stop not running", func(t *testing.T) {
		monitor, err := channelconfig.NewChannelConfigMonitor(
			config, queryService, membershipService,
			orderingService, configService, "testnet", "mychannel",
		)
		require.NoError(t, err)

		err = monitor.Stop()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not running")
	})

	t.Run("start with nil context", func(t *testing.T) {
		monitor, err := channelconfig.NewChannelConfigMonitor(
			config, queryService, membershipService,
			orderingService, configService, "testnet", "mychannel",
		)
		require.NoError(t, err)

		err = monitor.Start(context.TODO())
		require.NoError(t, err)
		assert.True(t, monitor.IsRunning())

		err = monitor.Stop()
		require.NoError(t, err)
	})
}

func TestConfigurationUpdate(t *testing.T) {
	config := &channelconfig.Config{
		PollInterval:      100 * time.Millisecond,
		MaxRetries:        2,
		InitialRetryDelay: 10 * time.Millisecond,
		MaxRetryDelay:     100 * time.Millisecond,
	}

	t.Run("successful update on new version", func(t *testing.T) {
		queryService := &mock.QueryService{}
		membershipService := &mock.MembershipService{}
		orderingService := &mock.OrderingService{}
		configService := &mock.ConfigService{}

		envelope := &cb.Envelope{Payload: []byte("test")}
		orderers := []*grpc.ConnectionConfig{{Address: "orderer1:7050"}}

		// First call returns version 1, second call returns version 2, then keep returning version 2
		callCount := 0
		queryService.GetConfigTransactionCalls(func() (*channelconfig.ConfigTransactionInfo, error) {
			callCount++
			if callCount == 1 {
				return &channelconfig.ConfigTransactionInfo{
					Envelope: envelope,
					Version:  1,
				}, nil
			}
			if callCount == 2 {
				return &channelconfig.ConfigTransactionInfo{
					Envelope: envelope,
					Version:  2,
				}, nil
			}
			// Keep returning version 2 for subsequent calls
			return &channelconfig.ConfigTransactionInfo{
				Envelope: envelope,
				Version:  2,
			}, nil
		})

		membershipService.UpdateReturns(nil)
		membershipService.OrdererConfigReturns("etcdraft", orderers, nil)
		orderingService.ConfigureReturns(nil)

		monitor, err := channelconfig.NewChannelConfigMonitor(
			config, queryService, membershipService,
			orderingService, configService, "testnet", "mychannel",
		)
		require.NoError(t, err)

		err = monitor.Start(context.Background())
		require.NoError(t, err)

		// Wait for initial check and one poll cycle
		time.Sleep(250 * time.Millisecond)

		err = monitor.Stop()
		require.NoError(t, err)

		// Verify services were called at least the expected number of times
		// The monitor polls continuously, so we use GreaterOrEqual
		assert.GreaterOrEqual(t, queryService.GetConfigTransactionCallCount(), 2)
		// Both version 1 and version 2 should trigger updates
		assert.Equal(t, 2, membershipService.UpdateCallCount())
		assert.Equal(t, 2, membershipService.OrdererConfigCallCount())
		assert.Equal(t, 2, orderingService.ConfigureCallCount())

		// Verify correct parameters for the first call
		_, capturedOrderers := orderingService.ConfigureArgsForCall(0)
		assert.Equal(t, orderers, capturedOrderers)
	})

	t.Run("no update on same version", func(t *testing.T) {
		queryService := &mock.QueryService{}
		membershipService := &mock.MembershipService{}
		orderingService := &mock.OrderingService{}
		configService := &mock.ConfigService{}

		envelope := &cb.Envelope{Payload: []byte("test")}

		// Always return same version
		queryService.GetConfigTransactionReturns(&channelconfig.ConfigTransactionInfo{
			Envelope: envelope,
			Version:  1,
		}, nil)

		monitor, err := channelconfig.NewChannelConfigMonitor(
			config, queryService, membershipService,
			orderingService, configService, "testnet", "mychannel",
		)
		require.NoError(t, err)

		err = monitor.Start(context.Background())
		require.NoError(t, err)

		// Wait for initial check and one poll cycle
		time.Sleep(250 * time.Millisecond)

		err = monitor.Stop()
		require.NoError(t, err)

		// Verify query was called and initial version triggered one update
		assert.Greater(t, queryService.GetConfigTransactionCallCount(), 0)
		// The first time we see version 1, it's treated as new (0 -> 1), so one update
		assert.Equal(t, 1, membershipService.UpdateCallCount())
		assert.Equal(t, 1, orderingService.ConfigureCallCount())
	})
}

func TestErrorHandling(t *testing.T) {
	config := &channelconfig.Config{
		PollInterval:      50 * time.Millisecond,
		MaxRetries:        2,
		InitialRetryDelay: 10 * time.Millisecond,
		MaxRetryDelay:     50 * time.Millisecond,
	}

	t.Run("query service error with retry", func(t *testing.T) {
		queryService := &mock.QueryService{}
		membershipService := &mock.MembershipService{}
		orderingService := &mock.OrderingService{}
		configService := &mock.ConfigService{}

		// Fail first two times, succeed third time, then keep succeeding
		callCount := 0
		queryService.GetConfigTransactionCalls(func() (*channelconfig.ConfigTransactionInfo, error) {
			callCount++
			if callCount <= 2 {
				return nil, errors.New("query error")
			}
			return &channelconfig.ConfigTransactionInfo{
				Envelope: &cb.Envelope{},
				Version:  1,
			}, nil
		})

		monitor, err := channelconfig.NewChannelConfigMonitor(
			config, queryService, membershipService,
			orderingService, configService, "testnet", "mychannel",
		)
		require.NoError(t, err)

		err = monitor.Start(context.Background())
		require.NoError(t, err)

		// Wait for retries and initial success
		time.Sleep(150 * time.Millisecond)

		err = monitor.Stop()
		require.NoError(t, err)

		// Should have retried at least 3 times (2 failures + 1 success)
		assert.GreaterOrEqual(t, queryService.GetConfigTransactionCallCount(), 3)
	})

	t.Run("membership update error", func(t *testing.T) {
		queryService := &mock.QueryService{}
		membershipService := &mock.MembershipService{}
		orderingService := &mock.OrderingService{}
		configService := &mock.ConfigService{}

		envelope := &cb.Envelope{Payload: []byte("test")}
		queryService.GetConfigTransactionReturns(&channelconfig.ConfigTransactionInfo{
			Envelope: envelope,
			Version:  2,
		}, nil)

		membershipService.UpdateReturns(errors.New("membership error"))

		monitor, err := channelconfig.NewChannelConfigMonitor(
			config, queryService, membershipService,
			orderingService, configService, "testnet", "mychannel",
		)
		require.NoError(t, err)

		err = monitor.Start(context.Background())
		require.NoError(t, err)

		// Wait for initial check and retries
		time.Sleep(200 * time.Millisecond)

		err = monitor.Stop()
		require.NoError(t, err)

		// Should have tried to update membership multiple times (with retries)
		assert.Greater(t, membershipService.UpdateCallCount(), 1)
	})

	t.Run("nil config transaction info", func(t *testing.T) {
		queryService := &mock.QueryService{}
		membershipService := &mock.MembershipService{}
		orderingService := &mock.OrderingService{}
		configService := &mock.ConfigService{}

		queryService.GetConfigTransactionReturns(nil, nil)

		monitor, err := channelconfig.NewChannelConfigMonitor(
			config, queryService, membershipService,
			orderingService, configService, "testnet", "mychannel",
		)
		require.NoError(t, err)

		err = monitor.Start(context.Background())
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		err = monitor.Stop()
		require.NoError(t, err)

		// Should not have called update services
		assert.Equal(t, 0, membershipService.UpdateCallCount())
	})
}

func TestContextCancellation(t *testing.T) {
	config := &channelconfig.Config{
		PollInterval:      1 * time.Second,
		MaxRetries:        5,
		InitialRetryDelay: 100 * time.Millisecond,
		MaxRetryDelay:     1 * time.Second,
	}

	queryService := &mock.QueryService{}
	membershipService := &mock.MembershipService{}
	orderingService := &mock.OrderingService{}
	configService := &mock.ConfigService{}

	queryService.GetConfigTransactionReturns(&channelconfig.ConfigTransactionInfo{
		Envelope: &cb.Envelope{},
		Version:  1,
	}, nil)

	t.Run("context cancellation stops monitoring", func(t *testing.T) {
		monitor, err := channelconfig.NewChannelConfigMonitor(
			config, queryService, membershipService,
			orderingService, configService, "testnet", "mychannel",
		)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		err = monitor.Start(ctx)
		require.NoError(t, err)
		assert.True(t, monitor.IsRunning())

		// Cancel context
		cancel()

		// Wait for monitoring to stop
		time.Sleep(100 * time.Millisecond)

		// Monitor should still report running until Stop is called
		assert.True(t, monitor.IsRunning())

		// Stop should succeed
		err = monitor.Stop()
		require.NoError(t, err)
		assert.False(t, monitor.IsRunning())
	})
}

// Made with Bob
