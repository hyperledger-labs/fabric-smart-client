/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
)

type provider struct {
	configService driver.ConfigService
	defaultName   string
}

type Provider interface {
	GetConfig(network string) (*config.Service, error)
}

func NewCore(config driver.ConfigService) (*core.Config, error) {
	return core.NewConfig(config)
}

func NewProvider(config driver.ConfigService) (Provider, error) {
	c, err := core.NewConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create config provider")
	}
	return &provider{
		defaultName:   c.DefaultName(),
		configService: config,
	}, nil
}

func (p *provider) GetConfig(network string) (*config.Service, error) {
	return config.NewService(p.configService, network, p.defaultName == network)
}
