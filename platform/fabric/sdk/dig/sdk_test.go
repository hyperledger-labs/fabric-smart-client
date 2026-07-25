/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/dig"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	viewconfig "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
)

func TestWiring(t *testing.T) {
	t.Parallel()
	assert.NoError(t, sdk.DryRunWiring(NewFrom, sdk.WithBool("fabric.enabled", true)))
}

// TestRegisterProcessorsForDrivers_NoOpOnEmptyConfig exercises registerProcessorsForDrivers
// directly (bypassing dig.DryRun, which never runs constructor bodies) to confirm it no-ops
// cleanly when the node started with zero Fabric networks configured (e.g. networks are only
// added later, at runtime, via core.FSNProvider.AddNetwork).
func TestRegisterProcessorsForDrivers_NoOpOnEmptyConfig(t *testing.T) {
	t.Parallel()
	p, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
`))
	require.NoError(t, err)

	cfg, err := core.NewConfig(p)
	require.NoError(t, err)
	require.Empty(t, cfg.Names())

	err = registerProcessorsForDrivers(struct {
		dig.In
		CoreConfig             *core.Config
		NetworkServiceProvider *fabric.NetworkServiceProvider
		Drivers                []core.NamedDriver `group:"fabric-platform-drivers"`
	}{
		CoreConfig: cfg,
	})
	require.NoError(t, err)
}
