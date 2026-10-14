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
// fabric.<network>.channels. An undeclared channel is an error, so that a misspelled channel
// name fails when the channel is opened.
func (c *channelConfigProvider) GetChannelConfig(network, channel string) (driver.ChannelConfig, error) {
	conf, err := c.configProvider.GetConfig(network)
	if err != nil {
		return nil, err
	}

	if channelConfig := conf.Channel(channel); channelConfig != nil {
		return channelConfig, nil
	}
	if channel == "" {
		// An empty name reaches here only when no channel is marked as the default.
		return nil, errors.Errorf("no default channel configured under fabric.%s.channels", network)
	}
	return nil, errors.Errorf("channel [%s] is not configured under fabric.%s.channels", channel, network)
}
