/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	viewconfig "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
)

const baseYAML = `
fabric:
  enabled: true
  default:
    default: true
    driver: generic
    channels:
      - Name: default-channel
`

func newTestProvider(t *testing.T, raw string) *viewconfig.Provider {
	t.Helper()
	p, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(raw))
	require.NoError(t, err)
	return p
}

func TestNewConfig(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, baseYAML)
	cfg, err := NewConfig(p)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"default"}, cfg.Names())
	assert.Equal(t, "default", cfg.DefaultName())

	fsnConfig, err := cfg.Config("default")
	require.NoError(t, err)
	assert.Equal(t, "generic", fsnConfig.Driver)
}

func TestNewConfig_RejectsMultipleExplicitDefaults(t *testing.T) {
	t.Parallel()
	_, err := NewConfig(newTestProvider(t, `
fabric:
  enabled: true
  network1:
    default: true
    driver: generic
    channels:
      - Name: mychannel
  network2:
    default: true
    driver: generic
    channels:
      - Name: mychannel
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple fabric networks are set as default")
}

func TestNewConfig_RejectsExplicitDefaultAlongsideNamedDefault(t *testing.T) {
	t.Parallel()
	// the network named "default" is implicitly the default, so a second network explicitly
	// marking itself as default is also ambiguous.
	_, err := NewConfig(newTestProvider(t, `
fabric:
  enabled: true
  default:
    driver: generic
    channels:
      - Name: default-channel
  network2:
    default: true
    driver: generic
    channels:
      - Name: mychannel
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple fabric networks are set as default")
}

func TestAddNetwork_RejectsSecondDefault(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, baseYAML)
	cfg, err := NewConfig(p)
	require.NoError(t, err)
	require.Equal(t, "default", cfg.DefaultName())

	err = cfg.AddNetwork([]byte(`
fabric:
  network2:
    default: true
    driver: generic
    channels:
      - Name: mychannel
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple fabric networks are set as default")
	// the live tree must be left untouched
	assert.ElementsMatch(t, []string{"default"}, cfg.Names())
	assert.Equal(t, "default", cfg.DefaultName())
}

func TestAddNetwork_Valid(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, baseYAML)
	cfg, err := NewConfig(p)
	require.NoError(t, err)

	err = cfg.AddNetwork([]byte(`
fabric:
  network2:
    driver: generic
    channels:
      - Name: mychannel
`))
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"default", "network2"}, cfg.Names())
	// the pre-existing default network is preserved
	assert.Equal(t, "default", cfg.DefaultName())

	fsnConfig, err := cfg.Config("network2")
	require.NoError(t, err)
	assert.Equal(t, "generic", fsnConfig.Driver)
}

func TestAddNetwork_RejectsExistingNetwork(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, baseYAML)
	cfg, err := NewConfig(p)
	require.NoError(t, err)

	err = cfg.AddNetwork([]byte(`
fabric:
  default:
    driver: generic
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// the live tree must be left untouched
	assert.ElementsMatch(t, []string{"default"}, cfg.Names())
}

func TestAddNetwork_RejectsBadNetworkName(t *testing.T) {
	t.Parallel()
	cases := []string{
		"Bad_Net",  // uppercase and underscore
		"1network", // must start with a letter
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := newTestProvider(t, baseYAML)
			cfg, err := NewConfig(p)
			require.NoError(t, err)

			err = cfg.AddNetwork([]byte(`
fabric:
  ` + name + `:
    driver: generic
`))
			require.Error(t, err)
			assert.ElementsMatch(t, []string{"default"}, cfg.Names())
		})
	}
}

func TestAddNetwork_EmptyNetworkName(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, baseYAML)
	cfg, err := NewConfig(p)
	require.NoError(t, err)

	err = cfg.AddNetwork([]byte(`
fabric:
  "":
    driver: generic
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
	assert.ElementsMatch(t, []string{"default"}, cfg.Names())
}

func TestAddNetwork_RejectsBadChannelName(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, baseYAML)
	cfg, err := NewConfig(p)
	require.NoError(t, err)

	err = cfg.AddNetwork([]byte(`
fabric:
  network2:
    driver: generic
    channels:
      - Name: Bad_Channel
`))
	require.Error(t, err)
	assert.ElementsMatch(t, []string{"default"}, cfg.Names())
}

func TestAddNetwork_RejectsDuplicateChannelWithinNetwork(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, baseYAML)
	cfg, err := NewConfig(p)
	require.NoError(t, err)

	err = cfg.AddNetwork([]byte(`
fabric:
  network2:
    driver: generic
    channels:
      - Name: mychannel
      - Name: mychannel
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate channel name")
	assert.ElementsMatch(t, []string{"default"}, cfg.Names())
}

func TestAddNetwork_AllowsSameChannelNameAcrossNetworks(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, baseYAML)
	cfg, err := NewConfig(p)
	require.NoError(t, err)

	err = cfg.AddNetwork([]byte(`
fabric:
  network2:
    driver: generic
    channels:
      - Name: default-channel
`))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"default", "network2"}, cfg.Names())
}

func TestAddNetwork_RunsCustomValidators(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, baseYAML)
	cfg, err := NewConfig(p)
	require.NoError(t, err)

	var seenNetwork string
	reject := NetworkConfigValidatorFunc(func(networkName string, configProvider ConfigProvider) error {
		seenNetwork = networkName
		var fsnConfig FSNConfig
		require.NoError(t, configProvider.UnmarshalKey("fabric."+networkName, &fsnConfig))
		if fsnConfig.Driver != "allowed-driver" {
			return errors.Errorf("driver [%s] is not allowed", fsnConfig.Driver)
		}
		return nil
	})

	err = cfg.AddNetwork([]byte(`
fabric:
  network2:
    driver: generic
    channels:
      - Name: mychannel
`), reject)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "custom validation failed")
	assert.Contains(t, err.Error(), "driver [generic] is not allowed")
	assert.Equal(t, "network2", seenNetwork)
	// the live tree must be left untouched
	assert.ElementsMatch(t, []string{"default"}, cfg.Names())

	err = cfg.AddNetwork([]byte(`
fabric:
  network3:
    driver: allowed-driver
    channels:
      - Name: mychannel
`), reject)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"default", "network3"}, cfg.Names())
}

func TestFSNProvider_AddNetwork(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, baseYAML)
	provider, err := NewFabricNetworkServiceProvider(p, nil, nil)
	require.NoError(t, err)

	err = provider.AddNetwork([]byte(`
fabric:
  network2:
    driver: generic
    channels:
      - Name: mychannel
`))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"default", "network2"}, provider.Names())
}

func TestFSNProvider_AddNetwork_RunsConfiguredValidators(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, baseYAML)
	reject := NetworkConfigValidatorFunc(func(networkName string, configProvider ConfigProvider) error {
		return errors.Errorf("network [%s] rejected by policy", networkName)
	})
	provider, err := NewFabricNetworkServiceProvider(p, nil, []NetworkConfigValidator{reject})
	require.NoError(t, err)

	err = provider.AddNetwork([]byte(`
fabric:
  network2:
    driver: generic
    channels:
      - Name: mychannel
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rejected by policy")
	assert.ElementsMatch(t, []string{"default"}, provider.Names())
}
