/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fns

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/dig"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	viewconfig "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
)

type fakeDriver struct{}

func (*fakeDriver) New(string, bool) (driver.FabricNetworkService, error) {
	return nil, nil
}

// fabricEnabledConfig builds a real *viewconfig.Provider from a minimal Fabric
// configuration. Using the production config implementation (rather than a hand-rolled
// fake) keeps the test exercising the same DynamicConfigService the wiring depends on.
func fabricEnabledConfig(t *testing.T) *viewconfig.Provider {
	t.Helper()
	p, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
`))
	require.NoError(t, err)
	return p
}

// TestNewProvider drives the fns.NewProvider dig constructor directly. dig.DryRun (used by
// TestWiring in the parent package) validates the graph without running constructor bodies,
// so a direct call is the only way to cover this one. With a network-less config and empty
// driver/validator groups it must return a usable *core.FSNProvider that reports no networks
// and has an empty driver map.
func TestNewProvider(t *testing.T) {
	t.Parallel()

	provider, err := NewProvider(struct {
		dig.In
		ConfigService core.DynamicConfigService
		Drivers       []core.NamedDriver            `group:"fabric-platform-drivers"`
		Validators    []core.NetworkConfigValidator `group:"fabric-network-config-validators"`
	}{
		ConfigService: fabricEnabledConfig(t),
	})
	require.NoError(t, err)
	require.NotNil(t, provider)
	require.Empty(t, provider.Names())
	require.Empty(t, provider.Drivers())
}

// TestNewProviderFoldsDrivers covers the branch that folds the injected driver group into the
// provider's internal driver map and asserts the resulting map state.
func TestNewProviderFoldsDrivers(t *testing.T) {
	t.Parallel()

	fakeD := &fakeDriver{}
	provider, err := NewProvider(struct {
		dig.In
		ConfigService core.DynamicConfigService
		Drivers       []core.NamedDriver            `group:"fabric-platform-drivers"`
		Validators    []core.NetworkConfigValidator `group:"fabric-network-config-validators"`
	}{
		ConfigService: fabricEnabledConfig(t),
		Drivers:       []core.NamedDriver{{Name: "generic", Driver: fakeD}},
	})
	require.NoError(t, err)
	require.NotNil(t, provider)
	namedDriver := core.NamedDriver{Name: "generic", Driver: fakeD}
	require.Equal(t, map[string]driver.Driver{"generic": namedDriver}, provider.Drivers())
}
