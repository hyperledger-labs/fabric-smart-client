/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
)

type configProvider struct {
	configService driver.ConfigService
	defaultName   string
}

type ConfigProvider interface {
	GetConfig(network string) (*config.Config, error)
}

func NewConfigProvider(config driver.ConfigService) (ConfigProvider, error) {
	c, err := core.NewConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create config provider")
	}
	return &configProvider{
		defaultName:   c.DefaultName(),
		configService: config,
	}, nil
}

func (p *configProvider) GetConfig(network string) (*config.Config, error) {
	return config.New(p.configService, network, p.defaultName == network)
}
