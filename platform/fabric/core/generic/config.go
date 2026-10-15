/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type channelConfigProvider struct {
	configProvider config.Provider
}

func NewChannelConfigProvider(nsp config.Provider) driver.ChannelConfigProvider {
	return &channelConfigProvider{configProvider: nsp}
}

// GetChannelConfig returns the configuration of the channel declared under
// fabric.<network>.channels, or of the default channel for an empty name. An undeclared
// channel is an error, so that a misspelled channel name fails when the channel is opened.
func (c *channelConfigProvider) GetChannelConfig(network, channel string) (driver.ChannelConfig, error) {
	conf, err := c.configProvider.GetConfig(network)
	if err != nil {
		return nil, err
	}

	if channel == "" {
		channel = conf.DefaultChannel()
		if channel == "" {
			return nil, errors.Errorf("no default channel configured under fabric.%s.channels", network)
		}
	}
	if channelConfig := conf.Channel(channel); channelConfig != nil {
		return channelConfig, nil
	}
	return nil, errors.Errorf("channel [%s] is not configured under fabric.%s.channels", channel, network)
}
