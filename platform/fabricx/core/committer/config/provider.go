/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
)

//go:generate counterfeiter -o mock/fabric_config_provider.go --fake-name FabricConfigProvider github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config.Provider
//go:generate counterfeiter -o mock/fabric_config_service.go --fake-name FabricConfigService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config.ConfigService

// Provider provides gRPC configuration for a given network.
type Provider struct {
	// configProvider is used to retrieve the configuration for a network.
	configProvider config.Provider
}

// NewProvider returns a new grpc.ServiceConfigProvider instance.
func NewProvider(configProvider config.Provider) *Provider {
	return &Provider{configProvider: configProvider}
}

// NotificationServiceConfig returns the configuration for the notification service for the specified network.
// It loads the configuration for the network and creates a Config instance.
func (c *Provider) NotificationServiceConfig(network string) (*Config, error) {
	// Load the specific configuration for this network
	cfg, err := c.configProvider.GetConfig(network)
	if err != nil {
		return nil, err
	}

	config, err := NewNotificationServiceConfig(cfg)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// QueryServiceConfig returns the configuration for the query service for the specified network.
// It loads the configuration for the network and creates a Config instance.
func (c *Provider) QueryServiceConfig(network string) (*Config, error) {
	// Load the specific configuration for this network
	cfg, err := c.configProvider.GetConfig(network)
	if err != nil {
		return nil, err
	}

	config, err := NewQueryServiceConfig(cfg)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// Made with Bob
