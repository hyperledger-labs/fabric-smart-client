/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

//go:generate counterfeiter -o mock/config_service.go -fake-name ConfigService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.ConfigService

const (
	// Default configuration values
	defaultPollInterval      = 1 * time.Second
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
func NewConfig(configService driver.ConfigService, network, channel string) (*Config, error) {
	prefix := buildConfigPrefix(network, channel)

	config := &Config{
		PollInterval:      defaultPollInterval,
		MaxRetries:        defaultMaxRetries,
		InitialRetryDelay: defaultInitialRetryDelay,
		MaxRetryDelay:     defaultMaxRetryDelay,
	}

	// Read poll interval if configured
	if configService.IsSet(prefix + "pollInterval") {
		config.PollInterval = configService.GetDuration(prefix + "pollInterval")
	}

	// Read max retries if configured
	if configService.IsSet(prefix + "maxRetries") {
		config.MaxRetries = configService.GetInt(prefix + "maxRetries")
	}

	// Read initial retry delay if configured
	if configService.IsSet(prefix + "initialRetryDelay") {
		config.InitialRetryDelay = configService.GetDuration(prefix + "initialRetryDelay")
	}

	// Read max retry delay if configured
	if configService.IsSet(prefix + "maxRetryDelay") {
		config.MaxRetryDelay = configService.GetDuration(prefix + "maxRetryDelay")
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, errors.WithMessagef(err, "invalid channel config monitor configuration")
	}

	return config, nil
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

// buildConfigPrefix constructs the configuration key prefix for the given network and channel
func buildConfigPrefix(network, channel string) string {
	if len(network) == 0 {
		network = "default"
	}
	return "fabric." + network + ".channels." + channel + ".configmonitor."
}

// Made with Bob
