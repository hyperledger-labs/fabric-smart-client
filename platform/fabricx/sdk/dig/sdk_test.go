/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	viewconfig "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

type failingSDK struct {
	common.SDK
	installErr error
	startErr   error
}

func (f *failingSDK) Install() error {
	if f.installErr != nil {
		return f.installErr
	}
	return f.SDK.Install()
}

func (f *failingSDK) Start(ctx context.Context) error {
	if f.startErr != nil {
		return f.startErr
	}
	return f.SDK.Start(ctx)
}

func TestWiring(t *testing.T) {
	t.Parallel()
	require.NoError(t, sdk.DryRunWiring(func(sdk common.SDK) *SDK { return NewFrom(fabric.NewFrom(sdk)) }, sdk.WithBool("fabric.enabled", true)))
}

func TestWiring_Disabled(t *testing.T) {
	t.Parallel()
	require.NoError(t, sdk.DryRunWiring(func(sdk common.SDK) *SDK { return NewFrom(fabric.NewFrom(sdk)) }, sdk.WithBool("fabric.enabled", false)))
}

func TestNewSDK(t *testing.T) {
	t.Parallel()
	registry := view.NewServiceProvider()
	cfgProvider, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: false
`))
	require.NoError(t, err)
	require.NoError(t, registry.RegisterService(cfgProvider))

	s := NewSDK(registry)
	require.NotNil(t, s)
	assert.False(t, s.FabricEnabled())
}

func TestSDK_Install_FabricDisabled(t *testing.T) {
	t.Parallel()
	cfgProvider, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: false
`))
	require.NoError(t, err)
	baseSDK := common.NewBaseSDK(sdk.NewContainer(), cfgProvider)
	s := NewFrom(baseSDK)
	assert.False(t, s.FabricEnabled())

	require.NoError(t, s.Install())
}

func TestSDK_Install_ParentError(t *testing.T) {
	t.Parallel()
	cfgProvider, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
`))
	require.NoError(t, err)
	baseSDK := common.NewBaseSDK(sdk.NewContainer(), cfgProvider)
	parentErr := errors.New("parent-install-failed")
	s := NewFrom(&failingSDK{SDK: baseSDK, installErr: parentErr})
	assert.True(t, s.FabricEnabled())

	err = s.Install()
	require.ErrorIs(t, err, parentErr)
}

func TestSDK_Start_FabricDisabled(t *testing.T) {
	t.Parallel()
	cfgProvider, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: false
`))
	require.NoError(t, err)
	baseSDK := common.NewBaseSDK(sdk.NewContainer(), cfgProvider)
	s := NewFrom(baseSDK)
	assert.False(t, s.FabricEnabled())

	require.NoError(t, s.Start(t.Context()))
}

func TestSDK_Start_MissingDependencies(t *testing.T) {
	t.Parallel()
	cfgProvider, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
`))
	require.NoError(t, err)
	baseSDK := common.NewBaseSDK(sdk.NewContainer(), cfgProvider)
	s := NewFrom(baseSDK)
	assert.True(t, s.FabricEnabled())

	err = s.Start(t.Context())
	require.ErrorContains(t, err, "missing")
}

func TestSDK_Start_Success(t *testing.T) {
	t.Parallel()
	cfgProvider, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
`))
	require.NoError(t, err)

	finalityProvider := finality.NewListenerManagerProvider(nil, nil)
	ledgerProvider := ledger.NewProvider(nil, nil)

	c := sdk.NewContainer()
	require.NoError(t, c.Provide(func() *finality.Provider { return finalityProvider }))
	require.NoError(t, c.Provide(func() *ledger.Provider { return ledgerProvider }))

	baseSDK := common.NewBaseSDK(c, cfgProvider)
	s := NewFrom(baseSDK)
	assert.True(t, s.FabricEnabled())

	require.NoError(t, s.Start(t.Context()))

	// Verify that ledgerProvider was initialized with context
	ctx, err := ledgerProvider.Context()
	require.NoError(t, err)
	require.Equal(t, t.Context(), ctx)
}

func TestSDK_Start_ParentError(t *testing.T) {
	t.Parallel()
	cfgProvider, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
`))
	require.NoError(t, err)

	finalityProvider := finality.NewListenerManagerProvider(nil, nil)
	ledgerProvider := ledger.NewProvider(nil, nil)

	c := sdk.NewContainer()
	require.NoError(t, c.Provide(func() *finality.Provider { return finalityProvider }))
	require.NoError(t, c.Provide(func() *ledger.Provider { return ledgerProvider }))

	baseSDK := common.NewBaseSDK(c, cfgProvider)
	parentErr := errors.New("parent-start-failed")
	s := NewFrom(&failingSDK{SDK: baseSDK, startErr: parentErr})
	assert.True(t, s.FabricEnabled())

	err = s.Start(t.Context())
	require.ErrorIs(t, err, parentErr)
}
