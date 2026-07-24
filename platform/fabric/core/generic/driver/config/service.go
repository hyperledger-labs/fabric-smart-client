/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// provider is the default Provider implementation. It keeps a reference to the shared
// configService and to the default network name resolved at construction time, and builds a
// generic/config.Service on demand for whichever network is requested.
type provider struct {
	configService driver.ConfigService
	defaultName   string
}

// ConfigService is the per-network configuration surface exposed by the generic Fabric driver:
// the common driver.ConfigService plus MSP- and resolver-related accessors.
type ConfigService interface {
	driver2.ConfigService
	Resolvers() ([]config.Resolver, error)
	MSPCacheSize() int
	DefaultMSP() string
	MSPs() ([]config.MSP, error)
}

// Provider hands out a ConfigService scoped to a given Fabric network name.
type Provider interface {
	GetConfig(network string) (ConfigService, error)
}

// NewCore builds a *core.Config from config, scanning its `fabric` key for the set of
// configured Fabric networks. config must also support merging new configuration into the
// live tree at runtime (core.DynamicConfigService), since the returned *core.Config's
// AddNetwork relies on it.
func NewCore(config core.DynamicConfigService) (*core.Config, error) {
	return core.NewConfig(config)
}

// NewProvider builds a Provider backed by config, resolving the default network name once at
// construction time via a *core.Config built from the same config.
func NewProvider(config core.DynamicConfigService) (Provider, error) {
	c, err := core.NewConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create config provider")
	}
	return &provider{
		defaultName:   c.DefaultName(),
		configService: config,
	}, nil
}

// GetConfig returns the ConfigService for the given Fabric network name.
func (p *provider) GetConfig(network string) (ConfigService, error) {
	return config.NewService(p.configService, network, p.defaultName == network)
}
