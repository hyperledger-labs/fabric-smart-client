/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/dig"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/identity"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	endorsermock "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
	viewconfig "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

// fabricEnabledConfig builds a real *viewconfig.Provider from a minimal Fabric configuration.
// The config declares no networks, so the constructors under test build their (network-less)
// singletons without touching a ledger.
func fabricEnabledConfig(t *testing.T) *viewconfig.Provider {
	t.Helper()
	p, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
`))
	require.NoError(t, err)
	return p
}

// fabricEnabledConfigWithNetwork builds a *viewconfig.Provider that configures a network with a channel.
func fabricEnabledConfigWithNetwork(t *testing.T) *viewconfig.Provider {
	t.Helper()
	p, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
  testnet:
    default: true
    driver: generic
    channels:
      - name: default-channel
        default: true
`))
	require.NoError(t, err)
	return p
}

type fakeVaultReader struct {
	driver2.LockedVaultReader
}

func (*fakeVaultReader) Done() error { return nil }

type fakeVaultStore struct {
	driver2.VaultStore
}

func (*fakeVaultStore) NewGlobalLockVaultReader(context.Context) (driver2.LockedVaultReader, error) {
	return &fakeVaultReader{}, nil
}

func (*fakeVaultStore) GetTxStatus(context.Context, driver2.TxID) (*driver2.TxStatus, error) {
	return nil, nil
}

func (*fakeVaultStore) Close() error { return nil }

type fakeDBDriver struct {
	endorseTxStore dbdriver.EndorseTxStore
	metadataStore  dbdriver.MetadataStore
	envelopeStore  dbdriver.EnvelopeStore
	vaultStore     driver2.VaultStore
}

func (f *fakeDBDriver) NewEndorseTx(driver3.PersistenceName, ...string) (dbdriver.EndorseTxStore, error) {
	return f.endorseTxStore, nil
}

func (f *fakeDBDriver) NewMetadata(driver3.PersistenceName, ...string) (dbdriver.MetadataStore, error) {
	return f.metadataStore, nil
}

func (f *fakeDBDriver) NewEnvelope(driver3.PersistenceName, ...string) (dbdriver.EnvelopeStore, error) {
	return f.envelopeStore, nil
}

func (f *fakeDBDriver) NewVault(driver3.PersistenceName, ...string) (driver2.VaultStore, error) {
	if f.vaultStore != nil {
		return f.vaultStore, nil
	}
	return &fakeVaultStore{}, nil
}

// fakeMultiplexedDriver builds a multiplexed driver over a fake DB driver by exercising
// the NewMultiplexedDriver dig constructor.
func fakeMultiplexedDriver(t *testing.T, cfg *viewconfig.Provider) multiplexed.Driver {
	t.Helper()
	return NewMultiplexedDriver(struct {
		dig.In
		Config  driver2.ConfigService
		Drivers []dbdriver.NamedDriver `group:"fabric-db-drivers" optional:"false"`
	}{
		Config: cfg,
		Drivers: []dbdriver.NamedDriver{
			{
				Name:   "memory",
				Driver: &fakeDBDriver{},
			},
		},
	})
}

// fakeOrdering implements both driver.Ordering and committer.OrderingService.
type fakeOrdering struct{}

func (*fakeOrdering) Broadcast(context.Context, any) error             { return nil }
func (*fakeOrdering) SetConsensusType(string) error                    { return nil }
func (*fakeOrdering) Configure(string, []*grpc.ConnectionConfig) error { return nil }

// fakeSigningIdentity implements driver.SigningIdentity.
type fakeSigningIdentity struct{}

func (*fakeSigningIdentity) Serialize() ([]byte, error)  { return []byte("fake-id"), nil }
func (*fakeSigningIdentity) Sign([]byte) ([]byte, error) { return []byte("fake-sig"), nil }

// fakeProcessorManager is a minimal implementation of driver.ProcessorManager.
type fakeProcessorManager struct{}

func (*fakeProcessorManager) AddProcessor(string, driver.Processor) error { return nil }
func (*fakeProcessorManager) SetDefaultProcessor(driver.Processor) error  { return nil }
func (*fakeProcessorManager) AddChannelProcessor(string, string, driver.Processor) error {
	return nil
}
func (*fakeProcessorManager) ProcessByID(context.Context, string, string) error { return nil }

// TestNewEndorserTransactionHandlerProvider covers the trivial-but-uncovered handler-provider
// constructor: dig.DryRun never runs constructor bodies, so a direct call is the only way to reach
// it. It must advertise the endorser-transaction header type with a non-nil handler factory.
func TestNewEndorserTransactionHandlerProvider(t *testing.T) {
	t.Parallel()

	res := NewEndorserTransactionHandlerProvider()
	require.Equal(t, common.HeaderType_ENDORSER_TRANSACTION, res.Type)
	require.NotNil(t, res.New)
}

// TestStoreProviders exercises NewMultiplexedDriver together with the three storage-provider
// constructors over a fake DB driver backend.
func TestStoreProviders(t *testing.T) {
	t.Parallel()

	cfg := fabricEnabledConfig(t)
	d := fakeMultiplexedDriver(t, cfg)

	endorseTxStore, err := NewEndorseTxStore(cfg, d)
	require.NoError(t, err)
	require.NotNil(t, endorseTxStore)

	metadataStore, err := NewMetadataStore(cfg, d)
	require.NoError(t, err)
	require.NotNil(t, metadataStore)

	envelopeStore, err := NewEnvelopeStore(cfg, d)
	require.NoError(t, err)
	require.NotNil(t, envelopeStore)
}

// TestNewDriver exercises the NewDriver dig constructor. gdriver.NewProvider (and the sig/identity
// sub-constructors it calls) only store their dependencies at construction, so typed-nil deps are
// enough to drive the body and confirm it returns the generic driver under the expected name.
func TestNewDriver(t *testing.T) {
	t.Parallel()

	d := NewDriver(struct {
		dig.In
		ConfigProvider  config.Provider
		MetricsProvider metrics.Provider
		EndpointService identity.EndpointService
		IDProvider      identity.ViewIdentityProvider
		KVS             *kvs.KVS
		AuditInfoKVS    driver2.AuditInfoStore
		SignerKVS       driver2.SignerInfoStore
		TracerProvider  tracing.Provider
		ChannelProvider generic.ChannelProvider        `name:"generic-channel-provider"`
		IdentityLoaders []identity.NamedIdentityLoader `group:"identity-loaders"`
	}{
		MetricsProvider: &disabled.Provider{},
	})

	require.Equal(t, config2.GenericDriver, d.Name)
	require.NotNil(t, d.Driver)
}

// TestNewChannelProvider exercises the NewChannelProvider dig constructor and asserts that the
// returned generic.ChannelProvider correctly wires the channel's vault, ledger, committer,
// delivery service, and RWSet loader.
func TestNewChannelProvider(t *testing.T) {
	t.Parallel()

	cfg := fabricEnabledConfigWithNetwork(t)
	d := fakeMultiplexedDriver(t, cfg)

	endorseTxStore, err := NewEndorseTxStore(cfg, d)
	require.NoError(t, err)

	metadataStore, err := NewMetadataStore(cfg, d)
	require.NoError(t, err)

	envelopeStore, err := NewEnvelopeStore(cfg, d)
	require.NoError(t, err)

	configProvider, err := config.NewProvider(cfg)
	require.NoError(t, err)

	confService, err := configProvider.GetConfig("testnet")
	require.NoError(t, err)

	channelProvider := NewChannelProvider(struct {
		dig.In
		ConfigProvider  config.Provider
		EnvelopeKVS     driver.EnvelopeStore
		MetadataKVS     driver.MetadataStore
		EndorseTxKVS    driver.EndorseTxStore
		Publisher       events.Publisher
		TracerProvider  tracing.Provider
		Drivers         multiplexed.Driver
		MetricsProvider metrics.Provider
	}{
		ConfigProvider:  configProvider,
		EnvelopeKVS:     envelopeStore,
		MetadataKVS:     metadataStore,
		EndorseTxKVS:    endorseTxStore,
		TracerProvider:  &noop.TracerProvider{},
		Drivers:         d,
		MetricsProvider: &disabled.Provider{},
	})
	require.NotNil(t, channelProvider)

	lmMock := &endorsermock.LocalMembership{}
	lmMock.DefaultSigningIdentityReturns(&fakeSigningIdentity{})

	pm := &fakeProcessorManager{}
	tmMock := &endorsermock.TransactionManager{}

	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.NameReturns("testnet")
	fnsMock.ConfigServiceReturns(confService)
	fnsMock.OrderingServiceReturns(&fakeOrdering{})
	fnsMock.LocalMembershipReturns(lmMock)
	fnsMock.ProcessorManagerReturns(pm)
	fnsMock.TransactionManagerReturns(tmMock)
	fnsMock.SignerServiceReturns(nil)

	ch, err := channelProvider.NewChannel(fnsMock, "default-channel", false)
	require.NoError(t, err)
	require.NotNil(t, ch)
	t.Cleanup(func() {
		require.NoError(t, ch.Close())
	})
	require.Equal(t, "default-channel", ch.Name())
	require.NotNil(t, ch.Vault())
	require.NotNil(t, ch.Ledger())
	require.NotNil(t, ch.Committer())
	require.NotNil(t, ch.Delivery())
	require.NotNil(t, ch.RWSetLoader())
}

// TestNewChannelProvider_InvalidOrdering verifies that NewChannel returns an error when
// the network's OrderingService does not implement committer.OrderingService.
func TestNewChannelProvider_InvalidOrdering(t *testing.T) {
	t.Parallel()

	cfg := fabricEnabledConfigWithNetwork(t)
	d := fakeMultiplexedDriver(t, cfg)

	endorseTxStore, err := NewEndorseTxStore(cfg, d)
	require.NoError(t, err)

	metadataStore, err := NewMetadataStore(cfg, d)
	require.NoError(t, err)

	envelopeStore, err := NewEnvelopeStore(cfg, d)
	require.NoError(t, err)

	configProvider, err := config.NewProvider(cfg)
	require.NoError(t, err)

	confService, err := configProvider.GetConfig("testnet")
	require.NoError(t, err)

	channelProvider := NewChannelProvider(struct {
		dig.In
		ConfigProvider  config.Provider
		EnvelopeKVS     driver.EnvelopeStore
		MetadataKVS     driver.MetadataStore
		EndorseTxKVS    driver.EndorseTxStore
		Publisher       events.Publisher
		TracerProvider  tracing.Provider
		Drivers         multiplexed.Driver
		MetricsProvider metrics.Provider
	}{
		ConfigProvider:  configProvider,
		EnvelopeKVS:     envelopeStore,
		MetadataKVS:     metadataStore,
		EndorseTxKVS:    endorseTxStore,
		TracerProvider:  &noop.TracerProvider{},
		Drivers:         d,
		MetricsProvider: &disabled.Provider{},
	})
	require.NotNil(t, channelProvider)

	lmMock := &endorsermock.LocalMembership{}
	lmMock.DefaultSigningIdentityReturns(&fakeSigningIdentity{})

	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.NameReturns("testnet")
	fnsMock.ConfigServiceReturns(confService)
	fnsMock.OrderingServiceReturns(&endorsermock.Ordering{})
	fnsMock.LocalMembershipReturns(lmMock)

	ch, err := channelProvider.NewChannel(fnsMock, "default-channel", false)
	require.ErrorContains(t, err, "ordering service is not a committer.OrderingService")
	require.Nil(t, ch)
}
