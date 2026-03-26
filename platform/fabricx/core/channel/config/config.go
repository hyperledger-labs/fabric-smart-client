/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
)

//go:generate counterfeiter -o mock/config_service.go -fake-name ConfigService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.ConfigService

const (
	configMonitorKey = "configMonitor"

	// Default configuration values
	defaultPollInterval      = 1 * time.Minute
	defaultMaxRetries        = 5
	defaultInitialRetryDelay = 1 * time.Second
	defaultMaxRetryDelay     = 5 * time.Minute
)

// Config holds the configuration parameters for the ChannelConfigMonitor service
type Config struct {
	// PollInterval is the duration between configuration checks
	PollInterval time.Duration

	// MaxRetries is the maximum number of retry attempts for failed operations
	MaxRetries int

	// InitialRetryDelay is the initial delay before the first retry
	InitialRetryDelay time.Duration

	// MaxRetryDelay is the maximum delay between retries
	MaxRetryDelay time.Duration
}

// NewConfig creates a new Config instance by reading from the provided ConfigService.
// It applies default values for any missing configuration parameters.
// The configService is assumed to be already rooted at the proper configuration location.
func NewConfig(configService driver.ConfigService) (*Config, error) {
	c := &Config{
		PollInterval:      defaultPollInterval,
		MaxRetries:        defaultMaxRetries,
		InitialRetryDelay: defaultInitialRetryDelay,
		MaxRetryDelay:     defaultMaxRetryDelay,
	}

	// Read poll interval if configured
	if configService.IsSet(config.Join(configMonitorKey, "pollInterval")) {
		c.PollInterval = configService.GetDuration("pollInterval")
	}

	// Read max retries if configured
	if configService.IsSet(config.Join(configMonitorKey, "maxRetries")) {
		c.MaxRetries = configService.GetInt("maxRetries")
	}

	// Read initial retry delay if configured
	if configService.IsSet(config.Join(configMonitorKey, "initialRetryDelay")) {
		c.InitialRetryDelay = configService.GetDuration("initialRetryDelay")
	}

	// Read max retry delay if configured
	if configService.IsSet(config.Join(configMonitorKey, "maxRetryDelay")) {
		c.MaxRetryDelay = configService.GetDuration("maxRetryDelay")
	}

	// Validate configuration
	if err := c.Validate(); err != nil {
		return nil, errors.WithMessagef(err, "invalid channel c monitor configuration")
	}

	return c, nil
}

// Validate checks that the configuration values are valid
func (c *Config) Validate() error {
	if c.PollInterval <= 0 {
		return errors.Errorf("pollInterval must be positive, got %v", c.PollInterval)
	}

	if c.MaxRetries < 0 {
		return errors.Errorf("maxRetries must be non-negative, got %d", c.MaxRetries)
	}

	if c.InitialRetryDelay <= 0 {
		return errors.Errorf("initialRetryDelay must be positive, got %v", c.InitialRetryDelay)
	}

	if c.MaxRetryDelay <= 0 {
		return errors.Errorf("maxRetryDelay must be positive, got %v", c.MaxRetryDelay)
	}

	if c.InitialRetryDelay > c.MaxRetryDelay {
		return errors.Errorf("initialRetryDelay (%v) must not exceed maxRetryDelay (%v)",
			c.InitialRetryDelay, c.MaxRetryDelay)
	}

	return nil
}
