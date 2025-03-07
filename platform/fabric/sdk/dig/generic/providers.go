/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	committer2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	gdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/identity"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ledger"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services"
	vdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/vault"
	"github.com/hyperledger/fabric-protos-go/common"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/dig"
)

type ChannelHandlerProviderResult struct {
	dig.Out
	RWSetPayloadHandlerProvider `group:"handler-providers"`
}

func NewEndorserTransactionHandlerProvider() ChannelHandlerProviderResult {
	return ChannelHandlerProviderResult{RWSetPayloadHandlerProvider: RWSetPayloadHandlerProvider{
		Type: common.HeaderType_ENDORSER_TRANSACTION,
		New:  rwset.NewEndorserTransactionHandler,
	}}
}

type RWSetPayloadHandlerProvider = digutils.HandlerProvider[common.HeaderType, func(network, channel string, v driver.RWSetInspector) driver.RWSetPayloadHandler]

func NewDriver(in struct {
	dig.In
	ConfigProvider  config.Provider
	MetricsProvider metrics.Provider
	EndpointService vdriver.EndpointService
	IdProvider      vdriver.IdentityProvider
	KVS             *kvs.KVS
	AuditInfoKVS    driver2.AuditInfoStore
	SignerKVS       driver2.SignerInfoStore
	TracerProvider  trace.TracerProvider
	ChannelProvider generic.ChannelProvider        `name:"generic-channel-provider"`
	IdentityLoaders []identity.NamedIdentityLoader `group:"identity-loaders"`
}) core.NamedDriver {
	d := core.NamedDriver{
		Name: config2.GenericDriver,
		Driver: gdriver.NewProvider(
			in.ConfigProvider,
			in.MetricsProvider,
			in.EndpointService,
			in.ChannelProvider,
			in.IdProvider,
			in.IdentityLoaders,
			in.SignerKVS,
			in.AuditInfoKVS,
			in.KVS,
			in.TracerProvider,
		),
	}
	return d
}

func NewChannelProvider(in struct {
	dig.In
	ConfigProvider  config.Provider
	EnvelopeKVS     driver.EnvelopeStore
	MetadataKVS     driver.MetadataStore
	EndorseTxKVS    driver.EndorseTxStore
	Publisher       events.Publisher
	Hasher          hash.Hasher
	TracerProvider  trace.TracerProvider
	Drivers         []dbdriver.NamedDriver `group:"db-drivers"`
	MetricsProvider metrics.Provider
}) generic.ChannelProvider {
	return generic.NewChannelProvider(
		in.EnvelopeKVS,
		in.MetadataKVS,
		in.EndorseTxKVS,
		in.Publisher,
		in.Hasher,
		in.TracerProvider,
		in.MetricsProvider,
		in.Drivers,
		func(_ string, configService driver.ConfigService, vaultStore driver2.VaultStore, metricsProvider metrics.Provider, tracerProvider trace.TracerProvider) (*vault.Vault, error) {
			cachedVault := vault2.NewCachedVault(vaultStore, configService.VaultTXStoreCacheSize())
			return vault.NewVault(cachedVault, metricsProvider, tracerProvider), nil
		},
		generic.NewChannelConfigProvider(in.ConfigProvider),
		committer2.NewFinalityListenerManagerProvider[driver.ValidationCode](in.TracerProvider),
		committer.NewSerialDependencyResolver(),
		func(
			channelName string,
			nw driver.FabricNetworkService,
			chaincodeManager driver.ChaincodeManager,
		) (driver.Ledger, error) {
			return ledger.New(
				channelName,
				chaincodeManager,
				nw.LocalMembership(),
				nw.ConfigService(),
				nw.TransactionManager(),
			), nil
		},
		func(
			channel string,
			nw driver.FabricNetworkService,
			envelopeService driver.EnvelopeService,
			transactionService driver.EndorserTransactionService,
			vault driver.RWSetInspector,
		) (driver.RWSetLoader, error) {
			return rwset.NewLoader(
				nw.Name(),
				channel,
				envelopeService,
				transactionService,
				nw.TransactionManager(),
				vault,
			), nil
		},
		func(
			nw driver.FabricNetworkService,
			channelConfig driver.ChannelConfig,
			vault driver.Vault,
			envelopeService driver.EnvelopeService,
			ledger driver.Ledger,
			rwsetLoaderService driver.RWSetLoader,
			eventsPublisher events.Publisher,
			channelMembershipService *membership.Service,
			fabricFinality committer.FabricFinality,
			dependencyResolver committer.DependencyResolver,
			quiet bool,
			listenerManager driver.ListenerManager,
			tracerProvider trace.TracerProvider,
			metricsProvider metrics.Provider,
		) (generic.CommitterService, error) {
			os, ok := nw.OrderingService().(committer.OrderingService)
			if !ok {
				return nil, errors.New("ordering service is not a committer.OrderingService")
			}

			return committer.New(
				nw.ConfigService(),
				channelConfig,
				vault,
				envelopeService,
				ledger,
				rwsetLoaderService,
				nw.ProcessorManager(),
				eventsPublisher,
				channelMembershipService,
				os,
				fabricFinality,
				nw.TransactionManager(),
				dependencyResolver,
				quiet,
				listenerManager,
				tracerProvider,
				metricsProvider,
			), nil
		},
		// delivery service constructor
		func(
			channel string,
			channelConfig driver.ChannelConfig,
			hasher hash.Hasher,
			networkName string,
			localMembership driver.LocalMembership,
			configService driver.ConfigService,
			peerManager delivery.Services,
			ledger driver.Ledger,
			waitForEventTimeout time.Duration,
			vault delivery.Vault,
			transactionManager driver.TransactionManager,
			callback driver.BlockCallback,
			tracerProvider trace.TracerProvider,
			metricsProvider metrics.Provider,
			acceptedHeaderTypes []common.HeaderType,
		) (generic.DeliveryService, error) {
			return delivery.NewService(
				channel,
				channelConfig,
				hasher,
				networkName,
				localMembership,
				configService,
				peerManager,
				ledger,
				waitForEventTimeout,
				vault,
				transactionManager,
				callback,
				tracerProvider,
				metricsProvider,
				acceptedHeaderTypes,
			)
		},
		true,
		[]common.HeaderType{common.HeaderType_ENDORSER_TRANSACTION},
	)
}

func NewEndorseTxStore(in struct {
	dig.In
	Config  vdriver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver.EndorseTxStore, error) {
	return services.NewDBBasedEndorseTxStore(in.Drivers, in.Config, "default")
}

func NewMetadataStore(in struct {
	dig.In
	Config  vdriver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver.MetadataStore, error) {
	return services.NewDBBasedMetadataStore(in.Drivers, in.Config, "default")
}

func NewEnvelopeStore(in struct {
	dig.In
	Config  vdriver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) (driver.EnvelopeStore, error) {
	return services.NewDBBasedEnvelopeStore(in.Drivers, in.Config, "default")
}
