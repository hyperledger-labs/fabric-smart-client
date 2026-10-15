/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config/mock"
	dconfig "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type stubConfigProvider struct {
	conf dconfig.ConfigService
}

func (p stubConfigProvider) GetConfig(string) (dconfig.ConfigService, error) { return p.conf, nil }

// TestGetChannelConfig runs against a real config.Service, whose channel map holds *Channel:
// an undeclared channel must surface as an error, not as a nil *Channel that panics on use.
func TestGetChannelConfig(t *testing.T) {
	t.Parallel()

	p := providerWithChannels(t, &config.Channel{Name: "ch1", Default: true}, &config.Channel{Name: "ch2"})

	c, err := p.GetChannelConfig("net", "ch2")
	require.NoError(t, err)
	require.Equal(t, "ch2", c.ID())

	c, err = p.GetChannelConfig("net", "")
	require.NoError(t, err)
	require.Equal(t, "ch1", c.ID())

	c, err = p.GetChannelConfig("net", "typo")
	require.EqualError(t, err, "channel [typo] is not configured under fabric.net.channels")
	require.Nil(t, c)

	c, err = providerWithChannels(t, &config.Channel{Name: "ch1"}).GetChannelConfig("net", "")
	require.EqualError(t, err, "no default channel configured under fabric.net.channels")
	require.Nil(t, c)
}

// providerWithChannels returns a provider for network "net" declaring the given channels.
func providerWithChannels(t *testing.T, channels ...*config.Channel) driver.ChannelConfigProvider {
	t.Helper()

	m := &mock.Configuration{}
	m.IsSetStub = func(key string) bool { return key == "fabric.net" }
	m.UnmarshalKeyStub = func(key string, rawVal any) error {
		if key == "fabric.net.channels" {
			*rawVal.(*[]*config.Channel) = channels
		}
		return nil
	}
	svc, err := config.NewService(m, "net", false)
	require.NoError(t, err)

	return NewChannelConfigProvider(stubConfigProvider{conf: svc})
}
