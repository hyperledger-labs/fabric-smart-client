/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"errors"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/dig"

	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	rwsetmock "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	generic2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig/generic"
	endorsermock "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	viewconfig "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

func TestWiring(t *testing.T) {
	t.Parallel()
	require.NoError(t, sdk.DryRunWiring(NewFrom, sdk.WithBool("fabric.enabled", true)))
}

func TestWiring_Disabled(t *testing.T) {
	t.Parallel()
	assert.NoError(t, sdk.DryRunWiring(NewFrom, sdk.WithBool("fabric.enabled", false)))
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

func TestSDK_Lifecycle_FabricDisabled(t *testing.T) {
	t.Parallel()
	cfgProvider, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: false
`))
	require.NoError(t, err)
	baseSDK := dig2.NewBaseSDK(sdk.NewContainer(), cfgProvider)
	s := NewFrom(baseSDK)
	assert.False(t, s.FabricEnabled())

	require.NoError(t, s.Install())
	require.NoError(t, s.Start(t.Context()))
	require.NoError(t, s.PostStart(t.Context()))
}

func TestSDK_PostStart_NoFNSProvider(t *testing.T) {
	t.Parallel()
	cfgProvider, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
`))
	require.NoError(t, err)
	baseSDK := dig2.NewBaseSDK(sdk.NewContainer(), cfgProvider)
	s := NewFrom(baseSDK)
	assert.True(t, s.FabricEnabled())

	err = s.PostStart(t.Context())
	require.ErrorContains(t, err, "no fabric network provider found")
}

func TestSDK_PostStart_Success(t *testing.T) {
	t.Parallel()
	cfgProvider, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
`))
	require.NoError(t, err)
	baseSDK := dig2.NewBaseSDK(sdk.NewContainer(), cfgProvider)
	s := NewFrom(baseSDK)

	fnsProvider, err := core.NewFabricNetworkServiceProvider(cfgProvider, nil, nil)
	require.NoError(t, err)
	s.fnsProvider = fnsProvider

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	// PostStart spawns a goroutine that stops fnsProvider when ctx is cancelled; t.Cleanup
	// cancels ctx so that goroutine unwinds instead of leaking past the test. fnsProvider is a
	// concrete *core.FSNProvider (not an interface), so its Stop() has no unit-observable effect
	// to assert on here — the shutdown wiring is covered by integration tests.
	require.NoError(t, s.PostStart(ctx))
}

func TestSDK_PostStart_StartError(t *testing.T) {
	t.Parallel()
	cfgProvider, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
  badnet:
    driver: non-existent-driver
`))
	require.NoError(t, err)
	baseSDK := dig2.NewBaseSDK(sdk.NewContainer(), cfgProvider)
	s := NewFrom(baseSDK)

	fnsProvider, err := core.NewFabricNetworkServiceProvider(cfgProvider, nil, nil)
	require.NoError(t, err)
	s.fnsProvider = fnsProvider

	err = s.PostStart(t.Context())
	require.ErrorContains(t, err, "failed starting fabric network service provider")
}

// configWithNetwork builds a *core.Config that has a network named "testnet" with driver "generic".
func configWithNetwork(t *testing.T) *core.Config {
	t.Helper()
	p, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
  testnet:
    default: true
    driver: generic
`))
	require.NoError(t, err)
	cfg, err := core.NewConfig(p)
	require.NoError(t, err)
	require.Equal(t, []string{"testnet"}, cfg.Names())
	return cfg
}

// configWithMultipleNetworks builds a *core.Config with two networks with different drivers.
func configWithMultipleNetworks(t *testing.T) *core.Config {
	t.Helper()
	p, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
  net1:
    default: true
    driver: generic
  net2:
    driver: other
`))
	require.NoError(t, err)
	cfg, err := core.NewConfig(p)
	require.NoError(t, err)
	return cfg
}

// emptyConfig builds a *core.Config with no Fabric networks.
func emptyConfig(t *testing.T) *core.Config {
	t.Helper()
	p, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
`))
	require.NoError(t, err)
	cfg, err := core.NewConfig(p)
	require.NoError(t, err)
	return cfg
}

// fakeProcessorManager is a minimal implementation of driver.ProcessorManager that records calls.
type fakeProcessorManager struct {
	defaultProcessor driver.Processor
	err              error
}

func (*fakeProcessorManager) AddProcessor(string, driver.Processor) error { return nil }

func (f *fakeProcessorManager) SetDefaultProcessor(p driver.Processor) error {
	if f.err != nil {
		return f.err
	}
	f.defaultProcessor = p
	return nil
}

func (*fakeProcessorManager) AddChannelProcessor(string, string, driver.Processor) error {
	return nil
}

func (*fakeProcessorManager) ProcessByID(context.Context, string, string) error { return nil }

// fakeConfigService is a minimal implementation of driver.ConfigService that only provides ChannelIDs.
type fakeConfigService struct {
	driver.ConfigService
	channelIDs []string
}

func (f *fakeConfigService) ChannelIDs() []string { return f.channelIDs }

// fakeDriverForFSN implements driver.Driver to inject a pre-built FNS into FSNProvider.
type fakeDriverForFSN struct {
	fns driver.FabricNetworkService
}

func (f *fakeDriverForFSN) New(string, bool) (driver.FabricNetworkService, error) {
	return f.fns, nil
}

// ---------------------------------------------------------------------------
// registerProcessorsForDrivers tests
// ---------------------------------------------------------------------------

// TestRegisterProcessorsForDrivers_NoOpOnEmptyConfig exercises registerProcessorsForDrivers
// directly (bypassing dig.DryRun, which never runs constructor bodies) to confirm it no-ops
// cleanly when the node started with zero Fabric networks configured (e.g. networks are only
// added later, at runtime, via core.FSNProvider.AddNetwork).
func TestRegisterProcessorsForDrivers_NoOpOnEmptyConfig(t *testing.T) {
	t.Parallel()
	cfg := emptyConfig(t)
	require.Empty(t, cfg.Names())

	err := registerProcessorsForDrivers(struct {
		dig.In
		CoreConfig             *core.Config
		NetworkServiceProvider *fabric.NetworkServiceProvider
		Drivers                []core.NamedDriver `group:"fabric-platform-drivers"`
	}{
		CoreConfig: cfg,
	})
	require.NoError(t, err)
}

// TestRegisterProcessorsForDrivers_WithNetwork exercises the happy path where the processor
// registration loops execute for a configured network. It uses a counterfeiter mock for the
// FNS provider and verifies that SetDefaultProcessor is called.
func TestRegisterProcessorsForDrivers_WithNetwork(t *testing.T) {
	t.Parallel()
	cfg := configWithNetwork(t)

	pm := &fakeProcessorManager{}

	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.NameReturns("testnet")
	fnsMock.ProcessorManagerReturns(pm)

	fnspMock := &endorsermock.FabricNetworkServiceProvider{}
	fnspMock.FabricNetworkServiceStub = func(_ string) (driver.FabricNetworkService, error) {
		return fnsMock, nil
	}
	fnspMock.DefaultNameReturns("testnet")
	fnspMock.NamesReturns([]string{"testnet"})

	nsp := fabric.NewNetworkServiceProvider(fnspMock, nil)

	err := registerProcessorsForDrivers(struct {
		dig.In
		CoreConfig             *core.Config
		NetworkServiceProvider *fabric.NetworkServiceProvider
		Drivers                []core.NamedDriver `group:"fabric-platform-drivers"`
	}{
		CoreConfig:             cfg,
		NetworkServiceProvider: nsp,
		Drivers:                []core.NamedDriver{{Name: "generic"}},
	})
	require.NoError(t, err)
	require.NotNil(t, pm.defaultProcessor, "SetDefaultProcessor must have been called")
}

// TestRegisterProcessorsForDrivers_DriverMismatch exercises the loop over drivers when a driver
// does not match the default network's configured driver. The mismatching driver must be skipped
// via 'continue' so that any subsequent matching driver in the list is still processed.
func TestRegisterProcessorsForDrivers_DriverMismatch(t *testing.T) {
	t.Parallel()
	cfg := configWithNetwork(t)

	pm := &fakeProcessorManager{}

	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.NameReturns("testnet")
	fnsMock.ProcessorManagerReturns(pm)

	fnspMock := &endorsermock.FabricNetworkServiceProvider{}
	fnspMock.FabricNetworkServiceStub = func(_ string) (driver.FabricNetworkService, error) {
		return fnsMock, nil
	}
	fnspMock.DefaultNameReturns("testnet")
	fnspMock.NamesReturns([]string{"testnet"})

	nsp := fabric.NewNetworkServiceProvider(fnspMock, nil)

	err := registerProcessorsForDrivers(struct {
		dig.In
		CoreConfig             *core.Config
		NetworkServiceProvider *fabric.NetworkServiceProvider
		Drivers                []core.NamedDriver `group:"fabric-platform-drivers"`
	}{
		CoreConfig:             cfg,
		NetworkServiceProvider: nsp,
		Drivers: []core.NamedDriver{
			{Name: "some-other-driver"},
			{Name: "generic"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, pm.defaultProcessor, "subsequent matching driver must be registered even if prior driver mismatched")
}

// TestRegisterProcessorsForDrivers_NoDrivers exercises the case with no drivers at all.
func TestRegisterProcessorsForDrivers_NoDrivers(t *testing.T) {
	t.Parallel()
	cfg := configWithNetwork(t)

	err := registerProcessorsForDrivers(struct {
		dig.In
		CoreConfig             *core.Config
		NetworkServiceProvider *fabric.NetworkServiceProvider
		Drivers                []core.NamedDriver `group:"fabric-platform-drivers"`
	}{
		CoreConfig: cfg,
		Drivers:    []core.NamedDriver{},
	})
	require.NoError(t, err)
}

func TestRegisterProcessorsForDrivers_DefaultFNSError(t *testing.T) {
	t.Parallel()
	cfg := configWithNetwork(t)

	fnspMock := &endorsermock.FabricNetworkServiceProvider{}
	fnspMock.FabricNetworkServiceStub = func(_ string) (driver.FabricNetworkService, error) {
		return nil, errors.New("default-fns-error")
	}
	fnspMock.DefaultNameReturns("testnet")
	fnspMock.NamesReturns([]string{"testnet"})

	nsp := fabric.NewNetworkServiceProvider(fnspMock, nil)

	err := registerProcessorsForDrivers(struct {
		dig.In
		CoreConfig             *core.Config
		NetworkServiceProvider *fabric.NetworkServiceProvider
		Drivers                []core.NamedDriver `group:"fabric-platform-drivers"`
	}{
		CoreConfig:             cfg,
		NetworkServiceProvider: nsp,
		Drivers:                []core.NamedDriver{{Name: "generic"}},
	})
	require.ErrorContains(t, err, "could not find default FNS")
}

// configWithTwoGenericNetworks builds a *core.Config with two networks both using generic driver.
func configWithTwoGenericNetworks(t *testing.T) *core.Config {
	t.Helper()
	p, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
  net1:
    default: true
    driver: generic
  net2:
    driver: generic
`))
	require.NoError(t, err)
	cfg, err := core.NewConfig(p)
	require.NoError(t, err)
	return cfg
}

func TestRegisterProcessorsForDrivers_NetworkFNSError(t *testing.T) {
	t.Parallel()
	cfg := configWithTwoGenericNetworks(t)

	pm := &fakeProcessorManager{}
	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.NameReturns("net1")
	fnsMock.ProcessorManagerReturns(pm)

	fnspMock := &endorsermock.FabricNetworkServiceProvider{}
	fnspMock.FabricNetworkServiceStub = func(id string) (driver.FabricNetworkService, error) {
		if id == "" || id == "net1" {
			return fnsMock, nil
		}
		return nil, errors.New("network-fns-error")
	}
	fnspMock.DefaultNameReturns("net1")
	fnspMock.NamesReturns([]string{"net1", "net2"})

	nsp := fabric.NewNetworkServiceProvider(fnspMock, nil)

	err := registerProcessorsForDrivers(struct {
		dig.In
		CoreConfig             *core.Config
		NetworkServiceProvider *fabric.NetworkServiceProvider
		Drivers                []core.NamedDriver `group:"fabric-platform-drivers"`
	}{
		CoreConfig:             cfg,
		NetworkServiceProvider: nsp,
		Drivers:                []core.NamedDriver{{Name: "generic"}},
	})
	require.ErrorContains(t, err, "could not find FNS [net2]")
}

func TestRegisterProcessorsForDrivers_SetDefaultProcessorError(t *testing.T) {
	t.Parallel()
	cfg := configWithNetwork(t)

	pm := &fakeProcessorManager{err: errors.New("set-processor-error")}

	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.NameReturns("testnet")
	fnsMock.ProcessorManagerReturns(pm)

	fnspMock := &endorsermock.FabricNetworkServiceProvider{}
	fnspMock.FabricNetworkServiceStub = func(_ string) (driver.FabricNetworkService, error) {
		return fnsMock, nil
	}
	fnspMock.DefaultNameReturns("testnet")
	fnspMock.NamesReturns([]string{"testnet"})

	nsp := fabric.NewNetworkServiceProvider(fnspMock, nil)

	err := registerProcessorsForDrivers(struct {
		dig.In
		CoreConfig             *core.Config
		NetworkServiceProvider *fabric.NetworkServiceProvider
		Drivers                []core.NamedDriver `group:"fabric-platform-drivers"`
	}{
		CoreConfig:             cfg,
		NetworkServiceProvider: nsp,
		Drivers:                []core.NamedDriver{{Name: "generic"}},
	})
	require.ErrorContains(t, err, "failed setting state processor for fabric network [testnet]")
}

func TestRegisterProcessorsForDrivers_MultiNetwork_SkipNonMatching(t *testing.T) {
	t.Parallel()
	cfg := configWithMultipleNetworks(t)

	pm := &fakeProcessorManager{}

	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.NameReturns("net1")
	fnsMock.ProcessorManagerReturns(pm)

	fnspMock := &endorsermock.FabricNetworkServiceProvider{}
	fnspMock.FabricNetworkServiceStub = func(_ string) (driver.FabricNetworkService, error) {
		return fnsMock, nil
	}
	fnspMock.DefaultNameReturns("net1")
	fnspMock.NamesReturns([]string{"net1", "net2"})

	nsp := fabric.NewNetworkServiceProvider(fnspMock, nil)

	err := registerProcessorsForDrivers(struct {
		dig.In
		CoreConfig             *core.Config
		NetworkServiceProvider *fabric.NetworkServiceProvider
		Drivers                []core.NamedDriver `group:"fabric-platform-drivers"`
	}{
		CoreConfig:             cfg,
		NetworkServiceProvider: nsp,
		Drivers:                []core.NamedDriver{{Name: "generic"}},
	})
	require.NoError(t, err)
	assert.NotNil(t, pm.defaultProcessor)
}

// ---------------------------------------------------------------------------
// registerRWSetLoaderHandlerProviders tests
// ---------------------------------------------------------------------------

// TestRegisterRWSetLoaderHandlerProviders_NoOpOnEmptyConfig exercises registerRWSetLoaderHandlerProviders
// directly to confirm it no-ops cleanly when no networks are configured.
func TestRegisterRWSetLoaderHandlerProviders_NoOpOnEmptyConfig(t *testing.T) {
	t.Parallel()
	p, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
`))
	require.NoError(t, err)
	cfg, err := core.NewConfig(p)
	require.NoError(t, err)

	err = registerRWSetLoaderHandlerProviders(struct {
		dig.In
		FSNProvider      *core.FSNProvider
		CoreConfig       *core.Config
		HandlerProviders []generic2.RWSetPayloadHandlerProvider `group:"handler-providers"`
	}{
		CoreConfig: cfg,
	})
	require.NoError(t, err)
}

// TestRegisterRWSetLoaderHandlerProviders_WithNetwork exercises the happy path where the handler
// providers are registered for each channel of each configured network. It creates a real
// FSNProvider with a mock FNS injected, and verifies AddHandlerProvider is called.
func TestRegisterRWSetLoaderHandlerProviders_WithNetwork(t *testing.T) {
	t.Parallel()
	p, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
  testnet:
    default: true
    driver: generic
`))
	require.NoError(t, err)
	cfg, err := core.NewConfig(p)
	require.NoError(t, err)

	loader := &rwsetmock.RWSetLoader{}

	channelMock := &endorsermock.Channel{}
	channelMock.RWSetLoaderReturns(loader)

	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.ConfigServiceReturns(&fakeConfigService{channelIDs: []string{"mychannel"}})
	fnsMock.ChannelReturns(channelMock, nil)

	fakeDriver := &fakeDriverForFSN{fns: fnsMock}
	fnsProvider, err := core.NewFabricNetworkServiceProvider(
		p,
		[]core.NamedDriver{{Name: "generic", Driver: fakeDriver}},
		nil,
	)
	require.NoError(t, err)

	err = registerRWSetLoaderHandlerProviders(struct {
		dig.In
		FSNProvider      *core.FSNProvider
		CoreConfig       *core.Config
		HandlerProviders []generic2.RWSetPayloadHandlerProvider `group:"handler-providers"`
	}{
		FSNProvider: fnsProvider,
		CoreConfig:  cfg,
		HandlerProviders: []generic2.RWSetPayloadHandlerProvider{
			{Type: common.HeaderType_ENDORSER_TRANSACTION},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, loader.AddHandlerProviderCallCount(), "AddHandlerProvider must have been called once")
}

func TestRegisterRWSetLoaderHandlerProviders_FNSError(t *testing.T) {
	t.Parallel()
	p, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
  testnet:
    default: true
    driver: unknown-driver
`))
	require.NoError(t, err)
	cfg, err := core.NewConfig(p)
	require.NoError(t, err)

	fnsProvider, err := core.NewFabricNetworkServiceProvider(p, nil, nil)
	require.NoError(t, err)

	err = registerRWSetLoaderHandlerProviders(struct {
		dig.In
		FSNProvider      *core.FSNProvider
		CoreConfig       *core.Config
		HandlerProviders []generic2.RWSetPayloadHandlerProvider `group:"handler-providers"`
	}{
		FSNProvider: fnsProvider,
		CoreConfig:  cfg,
	})
	require.ErrorContains(t, err, "could not find network service for testnet")
}

func TestRegisterRWSetLoaderHandlerProviders_ChannelError(t *testing.T) {
	t.Parallel()
	p, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
  testnet:
    default: true
    driver: generic
`))
	require.NoError(t, err)
	cfg, err := core.NewConfig(p)
	require.NoError(t, err)

	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.ConfigServiceReturns(&fakeConfigService{channelIDs: []string{"error-chan"}})
	fnsMock.ChannelReturns(nil, errors.New("channel-not-found"))

	fakeDriver := &fakeDriverForFSN{fns: fnsMock}
	fnsProvider, err := core.NewFabricNetworkServiceProvider(
		p,
		[]core.NamedDriver{{Name: "generic", Driver: fakeDriver}},
		nil,
	)
	require.NoError(t, err)

	err = registerRWSetLoaderHandlerProviders(struct {
		dig.In
		FSNProvider      *core.FSNProvider
		CoreConfig       *core.Config
		HandlerProviders []generic2.RWSetPayloadHandlerProvider `group:"handler-providers"`
	}{
		FSNProvider: fnsProvider,
		CoreConfig:  cfg,
	})
	require.ErrorContains(t, err, "could not find channel error-chan for network testnet")
}

func TestRegisterRWSetLoaderHandlerProviders_AddHandlerError(t *testing.T) {
	t.Parallel()
	p, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
  testnet:
    default: true
    driver: generic
`))
	require.NoError(t, err)
	cfg, err := core.NewConfig(p)
	require.NoError(t, err)

	loader := &rwsetmock.RWSetLoader{}
	loader.AddHandlerProviderReturns(errors.New("add-handler-failed"))

	channelMock := &endorsermock.Channel{}
	channelMock.RWSetLoaderReturns(loader)

	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.ConfigServiceReturns(&fakeConfigService{channelIDs: []string{"mychannel"}})
	fnsMock.ChannelReturns(channelMock, nil)

	fakeDriver := &fakeDriverForFSN{fns: fnsMock}
	fnsProvider, err := core.NewFabricNetworkServiceProvider(
		p,
		[]core.NamedDriver{{Name: "generic", Driver: fakeDriver}},
		nil,
	)
	require.NoError(t, err)

	err = registerRWSetLoaderHandlerProviders(struct {
		dig.In
		FSNProvider      *core.FSNProvider
		CoreConfig       *core.Config
		HandlerProviders []generic2.RWSetPayloadHandlerProvider `group:"handler-providers"`
	}{
		FSNProvider: fnsProvider,
		CoreConfig:  cfg,
		HandlerProviders: []generic2.RWSetPayloadHandlerProvider{
			{Type: common.HeaderType_ENDORSER_TRANSACTION},
		},
	})
	require.ErrorContains(t, err, "failed to add handler to channel mychannel")
}
