/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
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
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/endorsetx"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/envelope"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/metadata"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
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
	EndpointService identity.EndpointService
	IdProvider      identity.ViewIdentityProvider
	KVS             *kvs.KVS
	AuditInfoKVS    driver2.AuditInfoStore
	SignerKVS       driver2.SignerInfoStore
	TracerProvider  tracing.Provider
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
	TracerProvider  tracing.Provider
	Drivers         multiplexed.Driver
	MetricsProvider metrics.Provider
}) generic.ChannelProvider {
	flmProvider := committer2.NewFinalityListenerManagerProvider[driver.ValidationCode](in.TracerProvider)
	channelConfigProvider := generic.NewChannelConfigProvider(in.ConfigProvider)
	return generic.NewChannelProvider(
		in.ConfigProvider,
		in.EnvelopeKVS,
		in.MetadataKVS,
		in.EndorseTxKVS,
		in.Drivers,
		func(_ string, configService driver.ConfigService, vaultStore driver2.VaultStore) (*vault.Vault, error) {
			cachedVault := vault2.NewCachedVault(vaultStore, configService.VaultTXStoreCacheSize())
			return vault.NewVault(cachedVault, in.MetricsProvider, in.TracerProvider), nil
		},
		channelConfigProvider,
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
			channelName string,
			vault driver.Vault,
			envelopeService driver.EnvelopeService,
			ledger driver.Ledger,
			rwsetLoaderService driver.RWSetLoader,
			channelMembershipService driver.MembershipService,
			fabricFinality committer.FabricFinality,
			quiet bool,
		) (generic.CommitterService, error) {
			os, ok := nw.OrderingService().(committer.OrderingService)
			if !ok {
				return nil, errors.New("ordering service is not a committer.OrderingService")
			}
			channelConfig, err := channelConfigProvider.GetChannelConfig(nw.Name(), channelName)
			if err != nil {
				return nil, err
			}
			return committer.New(
				nw.ConfigService(),
				channelConfig,
				vault,
				envelopeService,
				ledger,
				rwsetLoaderService,
				nw.ProcessorManager(),
				in.Publisher,
				channelMembershipService,
				os,
				fabricFinality,
				nw.TransactionManager(),
				committer.NewSerialDependencyResolver(),
				quiet,
				flmProvider.NewManager(),
				in.TracerProvider,
				in.MetricsProvider,
			), nil
		},
		func(
			nw driver.FabricNetworkService,
			channel string,
			peerManager delivery.Services,
			ledger driver.Ledger,
			vault delivery.Vault,
			callback driver.BlockCallback,
		) (generic.DeliveryService, error) {
			channelConfig, err := channelConfigProvider.GetChannelConfig(nw.Name(), channel)
			if err != nil {
				return nil, err
			}
			return delivery.NewService(
				channel,
				channelConfig,
				nw.Name(),
				nw.LocalMembership(),
				nw.ConfigService(),
				peerManager,
				ledger,
				vault,
				nw.TransactionManager(),
				callback,
				in.TracerProvider,
				in.MetricsProvider,
				[]common.HeaderType{common.HeaderType_ENDORSER_TRANSACTION},
			)
		},
		func(channelName string) driver.MembershipService {
			return membership.NewService(channelName)
		},
		true)
}

func NewEndorseTxStore(config driver2.ConfigService, drivers multiplexed.Driver) (driver.EndorseTxStore, error) {
	return endorsetx.NewStore[driver.Key](config, drivers, "default")
}

func NewMetadataStore(config driver2.ConfigService, drivers multiplexed.Driver) (driver.MetadataStore, error) {
	return metadata.NewStore[driver.Key, driver.TransientMap](config, drivers, "default")
}

func NewEnvelopeStore(config driver2.ConfigService, drivers multiplexed.Driver) (driver.EnvelopeStore, error) {
	return envelope.NewStore[driver.Key](config, drivers, "default")
}

func NewMultiplexedDriver(in struct {
	dig.In
	Config  driver2.ConfigService
	Drivers []dbdriver.NamedDriver `group:"fabric-db-drivers" optional:"false"`
}) multiplexed.Driver {
	return multiplexed.NewDriver(in.Config, in.Drivers...)
}
