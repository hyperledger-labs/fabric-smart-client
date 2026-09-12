/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/dig"
	"google.golang.org/grpc"

	pkgerrors "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	config "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	identity "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/identity"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	multiplexed "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	endorsermock "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
	fabricx "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx"
	queryservice "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	finality "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
	ledger "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger"
	ledgermock "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger/mock"
	viewconfig "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	events "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	metrics "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	disabled "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	kvs "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
)

type fakeSigningIdentity struct{}

func (*fakeSigningIdentity) Serialize() ([]byte, error)  { return []byte("fake-id"), nil }
func (*fakeSigningIdentity) Sign([]byte) ([]byte, error) { return []byte("fake-sig"), nil }

type fakeListenerManager struct{}

func (*fakeListenerManager) AddFinalityListener(driver2.TxID, fdriver.FinalityListener) error {
	return nil
}

func (*fakeListenerManager) RemoveFinalityListener(driver2.TxID, fdriver.FinalityListener) error {
	return nil
}

type fakeListenerManagerProvider struct {
	err error
}

func (f *fakeListenerManagerProvider) NewManager(string, string) (finality.ListenerManager, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &fakeListenerManager{}, nil
}

type eventsPublisher struct{}

func (eventsPublisher) Publish(events.Event) {}

func fabricEnabledConfigWithNetwork(t *testing.T) *viewconfig.Provider {
	t.Helper()
	p, err := (&viewconfig.Provider{}).ProvideFromRaw([]byte(`
fabric:
  enabled: true
  testnet:
    default: true
    driver: fabricx
    configMonitor:
      maxRetries: 0
      pollInterval: 1m
    channels:
      - name: default-channel
        default: true
`))
	require.NoError(t, err)
	return p
}

func TestNewDriver(t *testing.T) {
	t.Parallel()

	d := NewDriver(struct {
		dig.In
		ConfigProvider  config.Provider
		MetricsProvider metrics.Provider
		EndpointService identity.EndpointService
		IDProvider      identity.ViewIdentityProvider
		KVS             *kvs.KVS
		SignerInfoStore driver2.SignerInfoStore
		AuditInfoStore  driver2.AuditInfoStore
		ChannelProvider ChannelProvider
		IdentityLoaders []identity.NamedIdentityLoader `group:"identity-loaders"`
	}{
		MetricsProvider: &disabled.Provider{},
	})

	require.Equal(t, fabricx.DriverName, d.Name)
	require.NotNil(t, d.Driver)
}

func TestNewRWSetLoader(t *testing.T) {
	t.Parallel()

	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.NameReturns("testnet")
	tmMock := &endorsermock.TransactionManager{}
	fnsMock.TransactionManagerReturns(tmMock)

	loader := NewRWSetLoader("testchannel", fnsMock, nil, nil, nil)
	require.NotNil(t, loader)

	err := loader.AddHandlerProvider(cb.HeaderType_MESSAGE, func(string, string, fdriver.RWSetInspector) fdriver.RWSetPayloadHandler {
		return nil
	})
	require.NoError(t, err)

	err = loader.AddHandlerProvider(cb.HeaderType_MESSAGE, func(string, string, fdriver.RWSetInspector) fdriver.RWSetPayloadHandler {
		return nil
	})
	require.ErrorContains(t, err, "already defined for header type")
}

func TestNewChannelProvider(t *testing.T) {
	t.Parallel()

	cfg := fabricEnabledConfigWithNetwork(t)
	configProvider, err := config.NewProvider(cfg)
	require.NoError(t, err)

	confService, err := configProvider.GetConfig("testnet")
	require.NoError(t, err)

	mockQSProvider := &ledgermock.QueryServiceProvider{}
	mockQS := &ledgermock.QueryService{}
	mockQSProvider.GetReturns(mockQS, nil)
	mockQS.GetConfigTransactionReturns(&queryservice.ConfigTransactionInfo{
		Envelope: &cb.Envelope{},
		Version:  1,
	}, nil)

	mockGRPCProvider := &ledgermock.GRPCClientProvider{}
	mockGRPCProvider.NotificationServiceClientReturns(&grpc.ClientConn{}, nil)
	ledgerProvider := ledger.NewProvider(mockGRPCProvider, mockQSProvider)
	ledgerProvider.Initialize(t.Context())

	channelProvider := NewChannelProvider(struct {
		dig.In
		ConfigProvider          config.Provider
		KVS                     *kvs.KVS
		LedgerProvider          *ledger.Provider
		Publisher               events.Publisher
		TracerProvider          trace.TracerProvider
		MetricsProvider         metrics.Provider
		QueryServiceProvider    queryservice.Provider
		ListenerManagerProvider finality.ListenerManagerProvider
		IdentityLoaders         []identity.NamedIdentityLoader `group:"identity-loaders"`
		EndpointService         identity.EndpointService
		IDProvider              identity.ViewIdentityProvider
		EnvelopeStore           fdriver.EnvelopeStore
		MetadataStore           fdriver.MetadataStore
		EndorseTxStore          fdriver.EndorseTxStore
		Drivers                 multiplexed.Driver
	}{
		ConfigProvider:          configProvider,
		LedgerProvider:          ledgerProvider,
		Publisher:               eventsPublisher{},
		TracerProvider:          noop.NewTracerProvider(),
		MetricsProvider:         &disabled.Provider{},
		QueryServiceProvider:    mockQSProvider,
		ListenerManagerProvider: &fakeListenerManagerProvider{},
	})
	require.NotNil(t, channelProvider)

	lmMock := &endorsermock.LocalMembership{}
	lmMock.DefaultSigningIdentityReturns(&fakeSigningIdentity{})

	tmMock := &endorsermock.TransactionManager{}

	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.NameReturns("testnet")
	fnsMock.ConfigServiceReturns(confService)
	fnsMock.OrderingServiceReturns(&endorsermock.Ordering{})
	fnsMock.LocalMembershipReturns(lmMock)
	fnsMock.TransactionManagerReturns(tmMock)

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

func TestNewChannelProvider_VaultError(t *testing.T) {
	t.Parallel()

	cfg := fabricEnabledConfigWithNetwork(t)
	configProvider, err := config.NewProvider(cfg)
	require.NoError(t, err)

	confService, err := configProvider.GetConfig("testnet")
	require.NoError(t, err)

	mockQSProvider := &ledgermock.QueryServiceProvider{}
	mockQSProvider.GetReturns(nil, pkgerrors.New("qs-get-failed"))

	mockGRPCProvider := &ledgermock.GRPCClientProvider{}
	mockGRPCProvider.NotificationServiceClientReturns(&grpc.ClientConn{}, nil)
	ledgerProvider := ledger.NewProvider(mockGRPCProvider, mockQSProvider)
	ledgerProvider.Initialize(t.Context())

	channelProvider := NewChannelProvider(struct {
		dig.In
		ConfigProvider          config.Provider
		KVS                     *kvs.KVS
		LedgerProvider          *ledger.Provider
		Publisher               events.Publisher
		TracerProvider          trace.TracerProvider
		MetricsProvider         metrics.Provider
		QueryServiceProvider    queryservice.Provider
		ListenerManagerProvider finality.ListenerManagerProvider
		IdentityLoaders         []identity.NamedIdentityLoader `group:"identity-loaders"`
		EndpointService         identity.EndpointService
		IDProvider              identity.ViewIdentityProvider
		EnvelopeStore           fdriver.EnvelopeStore
		MetadataStore           fdriver.MetadataStore
		EndorseTxStore          fdriver.EndorseTxStore
		Drivers                 multiplexed.Driver
	}{
		ConfigProvider:          configProvider,
		LedgerProvider:          ledgerProvider,
		Publisher:               eventsPublisher{},
		TracerProvider:          noop.NewTracerProvider(),
		MetricsProvider:         &disabled.Provider{},
		QueryServiceProvider:    mockQSProvider,
		ListenerManagerProvider: &fakeListenerManagerProvider{},
	})

	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.NameReturns("testnet")
	fnsMock.ConfigServiceReturns(confService)

	ch, err := channelProvider.NewChannel(fnsMock, "default-channel", false)
	require.ErrorContains(t, err, "failed creating vault for channel [default-channel]")
	require.Nil(t, ch)
}

func TestNewChannelProvider_LedgerError(t *testing.T) {
	t.Parallel()

	cfg := fabricEnabledConfigWithNetwork(t)
	configProvider, err := config.NewProvider(cfg)
	require.NoError(t, err)

	confService, err := configProvider.GetConfig("testnet")
	require.NoError(t, err)

	mockQSProvider := &ledgermock.QueryServiceProvider{}
	mockQS := &ledgermock.QueryService{}
	mockQSProvider.GetReturns(mockQS, nil)

	mockGRPCProvider := &ledgermock.GRPCClientProvider{}
	mockGRPCProvider.NotificationServiceClientReturns(&grpc.ClientConn{}, nil)
	// Intentionally do not call Initialize on ledgerProvider so NewLedger fails
	ledgerProvider := ledger.NewProvider(mockGRPCProvider, mockQSProvider)

	channelProvider := NewChannelProvider(struct {
		dig.In
		ConfigProvider          config.Provider
		KVS                     *kvs.KVS
		LedgerProvider          *ledger.Provider
		Publisher               events.Publisher
		TracerProvider          trace.TracerProvider
		MetricsProvider         metrics.Provider
		QueryServiceProvider    queryservice.Provider
		ListenerManagerProvider finality.ListenerManagerProvider
		IdentityLoaders         []identity.NamedIdentityLoader `group:"identity-loaders"`
		EndpointService         identity.EndpointService
		IDProvider              identity.ViewIdentityProvider
		EnvelopeStore           fdriver.EnvelopeStore
		MetadataStore           fdriver.MetadataStore
		EndorseTxStore          fdriver.EndorseTxStore
		Drivers                 multiplexed.Driver
	}{
		ConfigProvider:          configProvider,
		LedgerProvider:          ledgerProvider,
		Publisher:               eventsPublisher{},
		TracerProvider:          noop.NewTracerProvider(),
		MetricsProvider:         &disabled.Provider{},
		QueryServiceProvider:    mockQSProvider,
		ListenerManagerProvider: &fakeListenerManagerProvider{},
	})

	lmMock := &endorsermock.LocalMembership{}
	lmMock.DefaultSigningIdentityReturns(&fakeSigningIdentity{})

	tmMock := &endorsermock.TransactionManager{}

	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.NameReturns("testnet")
	fnsMock.ConfigServiceReturns(confService)
	fnsMock.LocalMembershipReturns(lmMock)
	fnsMock.TransactionManagerReturns(tmMock)

	ch, err := channelProvider.NewChannel(fnsMock, "default-channel", false)
	require.ErrorContains(t, err, "failed creating ledger for channel [default-channel]")
	require.Nil(t, ch)
}

func TestNewChannelProvider_DeliveryError(t *testing.T) {
	t.Parallel()

	cfg := fabricEnabledConfigWithNetwork(t)
	configProvider, err := config.NewProvider(cfg)
	require.NoError(t, err)

	confService, err := configProvider.GetConfig("testnet")
	require.NoError(t, err)

	mockQSProvider := &ledgermock.QueryServiceProvider{}
	mockQS := &ledgermock.QueryService{}
	mockQSProvider.GetReturns(mockQS, nil)

	mockGRPCProvider := &ledgermock.GRPCClientProvider{}
	mockGRPCProvider.NotificationServiceClientReturns(&grpc.ClientConn{}, nil)
	ledgerProvider := ledger.NewProvider(mockGRPCProvider, mockQSProvider)
	ledgerProvider.Initialize(t.Context())

	channelProvider := NewChannelProvider(struct {
		dig.In
		ConfigProvider          config.Provider
		KVS                     *kvs.KVS
		LedgerProvider          *ledger.Provider
		Publisher               events.Publisher
		TracerProvider          trace.TracerProvider
		MetricsProvider         metrics.Provider
		QueryServiceProvider    queryservice.Provider
		ListenerManagerProvider finality.ListenerManagerProvider
		IdentityLoaders         []identity.NamedIdentityLoader `group:"identity-loaders"`
		EndpointService         identity.EndpointService
		IDProvider              identity.ViewIdentityProvider
		EnvelopeStore           fdriver.EnvelopeStore
		MetadataStore           fdriver.MetadataStore
		EndorseTxStore          fdriver.EndorseTxStore
		Drivers                 multiplexed.Driver
	}{
		ConfigProvider:          configProvider,
		LedgerProvider:          ledgerProvider,
		Publisher:               eventsPublisher{},
		TracerProvider:          noop.NewTracerProvider(),
		MetricsProvider:         &disabled.Provider{},
		QueryServiceProvider:    mockQSProvider,
		ListenerManagerProvider: &fakeListenerManagerProvider{},
	})

	lmMock := &endorsermock.LocalMembership{}
	lmMock.DefaultSigningIdentityReturns(&fakeSigningIdentity{})

	tmMock := &endorsermock.TransactionManager{}

	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.NameReturns("non-existent-network")
	fnsMock.ConfigServiceReturns(confService)
	fnsMock.LocalMembershipReturns(lmMock)
	fnsMock.TransactionManagerReturns(tmMock)

	ch, err := channelProvider.NewChannel(fnsMock, "default-channel", false)
	require.ErrorContains(t, err, "failed creating delivery for channel [default-channel]")
	require.Nil(t, ch)
}

func TestNewChannelProvider_ListenerManagerError(t *testing.T) {
	t.Parallel()

	cfg := fabricEnabledConfigWithNetwork(t)
	configProvider, err := config.NewProvider(cfg)
	require.NoError(t, err)

	confService, err := configProvider.GetConfig("testnet")
	require.NoError(t, err)

	mockQSProvider := &ledgermock.QueryServiceProvider{}
	mockQS := &ledgermock.QueryService{}
	mockQSProvider.GetReturns(mockQS, nil)

	mockGRPCProvider := &ledgermock.GRPCClientProvider{}
	mockGRPCProvider.NotificationServiceClientReturns(&grpc.ClientConn{}, nil)
	ledgerProvider := ledger.NewProvider(mockGRPCProvider, mockQSProvider)
	ledgerProvider.Initialize(t.Context())

	channelProvider := NewChannelProvider(struct {
		dig.In
		ConfigProvider          config.Provider
		KVS                     *kvs.KVS
		LedgerProvider          *ledger.Provider
		Publisher               events.Publisher
		TracerProvider          trace.TracerProvider
		MetricsProvider         metrics.Provider
		QueryServiceProvider    queryservice.Provider
		ListenerManagerProvider finality.ListenerManagerProvider
		IdentityLoaders         []identity.NamedIdentityLoader `group:"identity-loaders"`
		EndpointService         identity.EndpointService
		IDProvider              identity.ViewIdentityProvider
		EnvelopeStore           fdriver.EnvelopeStore
		MetadataStore           fdriver.MetadataStore
		EndorseTxStore          fdriver.EndorseTxStore
		Drivers                 multiplexed.Driver
	}{
		ConfigProvider:          configProvider,
		LedgerProvider:          ledgerProvider,
		Publisher:               eventsPublisher{},
		TracerProvider:          noop.NewTracerProvider(),
		MetricsProvider:         &disabled.Provider{},
		QueryServiceProvider:    mockQSProvider,
		ListenerManagerProvider: &fakeListenerManagerProvider{err: pkgerrors.New("listener-manager-failed")},
	})

	lmMock := &endorsermock.LocalMembership{}
	lmMock.DefaultSigningIdentityReturns(&fakeSigningIdentity{})

	tmMock := &endorsermock.TransactionManager{}

	fnsMock := &endorsermock.FabricNetworkService{}
	fnsMock.NameReturns("testnet")
	fnsMock.ConfigServiceReturns(confService)
	fnsMock.LocalMembershipReturns(lmMock)
	fnsMock.TransactionManagerReturns(tmMock)

	ch, err := channelProvider.NewChannel(fnsMock, "default-channel", false)
	require.ErrorContains(t, err, "failed creating listener manager for channel [default-channel]")
	require.Nil(t, ch)
}
