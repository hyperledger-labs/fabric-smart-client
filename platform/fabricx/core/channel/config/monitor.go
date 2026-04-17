/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"context"
	"sync"
	"time"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

var logger = logging.MustGetLogger()

// QueryService defines the interface for querying configuration transactions
//
//go:generate counterfeiter -o mock/query_service.go -fake-name QueryService . QueryService
type QueryService interface {
	// GetConfigTransaction retrieves the latest configuration transaction for the channel
	GetConfigTransaction() (*queryservice.ConfigTransactionInfo, error)
}

// MembershipService defines the interface for updating channel membership
//
//go:generate counterfeiter -o mock/membership_service.go -fake-name MembershipService . MembershipService
type MembershipService interface {
	// Update updates the membership service with a new configuration envelope
	Update(env *cb.Envelope) error

	// OrdererConfig extracts orderer configuration from the current channel config
	OrdererConfig(cs driver.ConfigService) (string, []*grpc.ConnectionConfig, error)
}

// OrderingService defines the interface for updating ordering service configuration
//
//go:generate counterfeiter -o mock/ordering_service.go -fake-name OrderingService . OrderingService
type OrderingService interface {
	// Configure updates the ordering service with new consensus type and orderer endpoints
	Configure(consensusType string, orderers []*grpc.ConnectionConfig) error
}

// ChannelConfigMonitor implements the monitor
type ChannelConfigMonitor struct {
	config            *Config
	queryService      QueryService
	membershipService MembershipService
	orderingService   OrderingService
	configService     driver.ConfigService
	network           string
	channel           string

	// State management
	mu          sync.RWMutex
	running     bool
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	lastVersion int64
}

// NewChannelConfigMonitor creates a new ChannelConfigMonitor service instance
func NewChannelConfigMonitor(
	config *Config,
	queryService QueryService,
	membershipService MembershipService,
	orderingService OrderingService,
	configService driver.ConfigService,
	network string,
	channel string,
) (*ChannelConfigMonitor, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}
	if queryService == nil {
		return nil, errors.New("queryService cannot be nil")
	}
	if membershipService == nil {
		return nil, errors.New("membershipService cannot be nil")
	}
	if orderingService == nil {
		return nil, errors.New("orderingService cannot be nil")
	}
	if configService == nil {
		return nil, errors.New("configService cannot be nil")
	}
	if len(channel) == 0 {
		return nil, errors.New("channel name cannot be empty")
	}

	return &ChannelConfigMonitor{
		config:            config,
		queryService:      queryService,
		membershipService: membershipService,
		orderingService:   orderingService,
		configService:     configService,
		network:           network,
		channel:           channel,
		running:           false,
		lastVersion:       -1,
	}, nil
}

// Start begins monitoring for configuration changes
func (s *ChannelConfigMonitor) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return errors.New("monitor is already running")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.running = true

	s.wg.Add(1)
	go s.monitorLoop()

	logger.Debugf("Channel config monitor started for [%s:%s]", s.network, s.channel)
	return nil
}

// Stop halts the monitoring process
func (s *ChannelConfigMonitor) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return errors.New("monitor is not running")
	}

	s.cancel()
	s.running = false
	s.mu.Unlock()

	// Wait for the monitoring goroutine to finish
	s.wg.Wait()

	logger.Debugf("Channel config monitor stopped for [%s:%s]", s.network, s.channel)
	return nil
}

// IsRunning returns true if the monitor is currently running
func (s *ChannelConfigMonitor) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// monitorLoop is the main monitoring loop that runs in a goroutine
func (s *ChannelConfigMonitor) monitorLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.PollInterval)
	defer ticker.Stop()

	logger.Debugf("Monitor loop started for [%s:%s], poll interval: %v",
		s.network, s.channel, s.config.PollInterval)

	// Perform initial check immediately
	if err := s.checkAndUpdate(); err != nil {
		logger.Errorf("Initial config check failed for [%s:%s]: %v", s.network, s.channel, err)
	}

	for {
		select {
		case <-s.ctx.Done():
			logger.Debugf("Monitor loop stopping for [%s:%s]: context cancelled", s.network, s.channel)
			return
		case <-ticker.C:
			if err := s.checkAndUpdate(); err != nil {
				logger.Errorf("Config check failed for [%s:%s]: %v", s.network, s.channel, err)
			}
		}
	}
}

// checkAndUpdate checks for configuration changes and applies updates if needed
func (s *ChannelConfigMonitor) checkAndUpdate() error {
	logger.Debugf("Checking for config updates on [%s:%s]", s.network, s.channel)

	// Wrap the operation with retry logic
	return s.retryWithBackoff(func() error {
		// Query for the latest configuration transaction
		configInfo, err := s.queryService.GetConfigTransaction()
		if err != nil {
			return errors.Wrap(err, "failed to get config transaction")
		}

		if configInfo == nil || configInfo.Envelope == nil {
			return errors.New("received nil config transaction info")
		}

		// Check if this is a new configuration
		if int64(configInfo.Version) <= s.lastVersion {
			logger.Debugf("No new config for [%s:%s], current version: %d", s.network, s.channel, configInfo.Version)
			return nil
		}

		logger.Debugf("New config detected for [%s:%s], version: %d -> %d",
			s.network, s.channel, s.lastVersion, configInfo.Version)

		// Apply the configuration update
		if err := s.applyConfigUpdate(configInfo.Envelope); err != nil {
			return errors.Wrap(err, "failed to apply config update")
		}

		// Update the last processed config version
		s.lastVersion = int64(configInfo.Version)
		logger.Debugf("Config update applied successfully for [%s:%s], new version: %d",
			s.network, s.channel, configInfo.Version)

		return nil
	})
}

// applyConfigUpdate applies the configuration update to membership and ordering services
func (s *ChannelConfigMonitor) applyConfigUpdate(envelope *cb.Envelope) error {
	// Update membership service
	if err := s.membershipService.Update(envelope); err != nil {
		return errors.Wrap(err, "failed to update membership service")
	}
	logger.Debugf("Membership service updated for [%s:%s]", s.network, s.channel)

	// Extract orderer configuration from membership service
	consensusType, orderers, err := s.membershipService.OrdererConfig(s.configService)
	if err != nil {
		return errors.Wrap(err, "failed to extract orderer config")
	}

	// Update ordering service
	if err := s.orderingService.Configure(consensusType, orderers); err != nil {
		return errors.Wrap(err, "failed to update ordering service")
	}
	logger.Debugf("Ordering service updated for [%s:%s], consensus: %s, orderers: %d",
		s.network, s.channel, consensusType, len(orderers))

	return nil
}

// retryWithBackoff executes the given operation with exponential backoff retry logic
func (s *ChannelConfigMonitor) retryWithBackoff(operation func() error) error {
	var lastErr error
	delay := s.config.InitialRetryDelay

	for attempt := 0; attempt <= s.config.MaxRetries; attempt++ {
		// Check if context is cancelled before attempting
		select {
		case <-s.ctx.Done():
			return errors.New("operation cancelled")
		default:
		}

		// Execute the operation
		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// If this was the last attempt, return the error
		if attempt == s.config.MaxRetries {
			break
		}

		// Log the retry attempt
		logger.Warnf("Operation failed for [%s:%s] (attempt %d/%d): %v, retrying in %v",
			s.network, s.channel, attempt+1, s.config.MaxRetries+1, err, delay)

		// Wait before retrying, respecting context cancellation
		select {
		case <-s.ctx.Done():
			return errors.New("operation cancelled during retry")
		case <-time.After(delay):
		}

		// Calculate next delay with exponential backoff
		delay *= 2
		if delay > s.config.MaxRetryDelay {
			delay = s.config.MaxRetryDelay
		}
	}

	return errors.Wrapf(lastErr, "operation failed after %d attempts", s.config.MaxRetries+1)
}
