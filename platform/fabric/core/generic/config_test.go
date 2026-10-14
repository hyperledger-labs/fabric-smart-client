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
)

type stubConfigProvider struct {
	conf dconfig.ConfigService
}

func (p stubConfigProvider) GetConfig(string) (dconfig.ConfigService, error) { return p.conf, nil }

// TestGetChannelConfig runs against a real config.Service, whose channel map holds *Channel:
// an undeclared channel must surface as an error, not as a nil *Channel that panics on use.
func TestGetChannelConfig(t *testing.T) {
	t.Parallel()

	m := &mock.Configuration{}
	m.IsSetStub = func(key string) bool { return key == "fabric.net" }
	m.UnmarshalKeyStub = func(key string, rawVal any) error {
		if key == "fabric.net.channels" {
			*rawVal.(*[]*config.Channel) = []*config.Channel{{Name: "ch1", Default: true}}
		}
		return nil
	}
	svc, err := config.NewService(m, "net", false)
	require.NoError(t, err)
	p := NewChannelConfigProvider(stubConfigProvider{conf: svc})

	c, err := p.GetChannelConfig("net", "ch1")
	require.NoError(t, err)
	require.Equal(t, "ch1", c.ID())

	c, err = p.GetChannelConfig("net", "typo")
	require.EqualError(t, err, "channel [typo] is not configured under fabric.net.channels")
	require.Nil(t, c)

	_, err = p.GetChannelConfig("net", "")
	require.EqualError(t, err, "no default channel configured under fabric.net.channels")
}
