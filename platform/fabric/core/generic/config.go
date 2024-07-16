/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/config"
)

type channelConfigProvider struct {
	configProvider config.Provider
}

func NewChannelConfigProvider(nsp config.Provider) driver.ChannelConfigProvider {
	return &channelConfigProvider{configProvider: nsp}
}

func (c *channelConfigProvider) GetChannelConfig(network, channel string) (driver.ChannelConfig, error) {
	conf, err := c.configProvider.GetConfig(network)
	if err != nil {
		return nil, err
	}

	//lint:ignore SA4023 Config can be missing
	if channelConfig := conf.Channel(channel); channelConfig != nil {
		return channelConfig, nil
	}
	return conf.NewDefaultChannelConfig(channel), nil
}
