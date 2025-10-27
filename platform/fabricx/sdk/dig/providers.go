/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	fcommitter "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/identity"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ordering"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx"
	committer2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/membership"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-committer/api/types"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/dig"
)

type P2PCommunicationType = string

const (
	FabricxDriverName                      = "fabricx"
	WebSocket         P2PCommunicationType = "websocket"
)

func NewDriver(in struct {
	dig.In
	ConfigProvider  config.Provider
	MetricsProvider metrics.Provider
	EndpointService identity.EndpointService
	IdProvider      identity.ViewIdentityProvider
	KVS             *kvs.KVS
	SignerInfoStore driver.SignerInfoStore
	AuditInfoStore  driver.AuditInfoStore
	ChannelProvider ChannelProvider
	IdentityLoaders []identity.NamedIdentityLoader `group:"identity-loaders"`
},
) core.NamedDriver {
	d := core.NamedDriver{
		Name: FabricxDriverName,
		Driver: fabricx.NewProvider(
			in.ConfigProvider,
			in.MetricsProvider,
			in.EndpointService,
			in.ChannelProvider,
			in.IdProvider,
			in.IdentityLoaders,
			in.KVS,
			in.SignerInfoStore,
			in.AuditInfoStore,
		),
	}
	return d
}

type ChannelProvider generic.ChannelProvider

func NewChannelProvider(in struct {
	dig.In
	ConfigProvider          config.Provider
	KVS                     *kvs.KVS
	LedgerProvider          ledger.Provider
	Publisher               events.Publisher
	BlockDispatcherProvider *ledger.BlockDispatcherProvider
	TracerProvider          trace.TracerProvider
	MetricsProvider         metrics.Provider
	QueryServiceProvider    queryservice.Provider
	IdentityLoaders         []identity.NamedIdentityLoader `group:"identity-loaders"`
	EndpointService         identity.EndpointService
	IdProvider              identity.ViewIdentityProvider
	EnvelopeStore           fdriver.EnvelopeStore
	MetadataStore           fdriver.MetadataStore
	EndorseTxStore          fdriver.EndorseTxStore
	Drivers                 multiplexed.Driver
},
) generic.ChannelProvider {
	channelConfigProvider := generic.NewChannelConfigProvider(in.ConfigProvider)
	flmProvider := committer.NewFinalityListenerManagerProvider[fdriver.ValidationCode](in.TracerProvider)
	return generic.NewChannelProvider(
		in.ConfigProvider,
		in.EnvelopeStore,
		in.MetadataStore,
		in.EndorseTxStore,
		in.Drivers,
		func(channelName string, configService fdriver.ConfigService, vaultStore driver.VaultStore) (*vault2.Vault, error) {
			return vault.New(configService, vaultStore, channelName, in.QueryServiceProvider, in.MetricsProvider, in.TracerProvider)
		},
		channelConfigProvider,
		func(channelName string, nw fdriver.FabricNetworkService, chaincodeManager fdriver.ChaincodeManager) (fdriver.Ledger, error) {
			return in.LedgerProvider.NewLedger(nw.Name(), channelName)
		},
		func(channel string, nw fdriver.FabricNetworkService, envelopeService fdriver.EnvelopeService, transactionService fdriver.EndorserTransactionService, vault fdriver.RWSetInspector) (fdriver.RWSetLoader, error) {
			return NewRWSetLoader(channel, nw, envelopeService, transactionService, vault), nil
		},
		func(nw fdriver.FabricNetworkService, channel string, vault fdriver.Vault, envelopeService fdriver.EnvelopeService, ledger fdriver.Ledger, rwsetLoaderService fdriver.RWSetLoader, channelMembershipService fdriver.MembershipService, fabricFinality fcommitter.FabricFinality, quiet bool) (generic.CommitterService, error) {
			channelConfig, err := channelConfigProvider.GetChannelConfig(nw.Name(), channel)
			if err != nil {
				return nil, err
			}
			return NewCommitter(nw, channelConfig, vault, envelopeService, ledger, rwsetLoaderService, in.Publisher, channelMembershipService, fabricFinality, fcommitter.NewSerialDependencyResolver(), quiet, flmProvider.NewManager(), in.TracerProvider, in.MetricsProvider)
		},
		// delivery service constructor
		func(
			nw fdriver.FabricNetworkService,
			channel string,
			peerManager delivery.Services,
			ledger fdriver.Ledger,
			vault delivery.Vault,
			callback fdriver.BlockCallback,
		) (generic.DeliveryService, error) {
			// we inject here the block dispatcher and the callback
			// note that once the committer queryservice/notification service is available, we will remove the
			// local commit-pipeline and delivery service
			dispatcher, err := in.BlockDispatcherProvider.GetBlockDispatcher(nw.Name(), channel)
			if err != nil {
				return nil, err
			}
			channelConfig, err := channelConfigProvider.GetChannelConfig(nw.Name(), channel)
			if err != nil {
				return nil, err
			}
			dispatcher.AddCallback(callback)

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
				dispatcher.OnBlock,
				in.TracerProvider,
				in.MetricsProvider,
				[]cb.HeaderType{cb.HeaderType_MESSAGE},
			)
		},
		// membership service
		func(channelName string) fdriver.MembershipService {
			return membership.NewService(channelName)
		},
		false,
	)
}

func NewRWSetLoader(channel string, nw fdriver.FabricNetworkService, envelopeService fdriver.EnvelopeService, transactionService fdriver.EndorserTransactionService, vault fdriver.RWSetInspector) fdriver.RWSetLoader {
	return rwset.NewLoader(nw.Name(), channel, envelopeService, transactionService, nw.TransactionManager(), vault)
}

func NewCommitter(nw fdriver.FabricNetworkService, channelConfig fdriver.ChannelConfig, vault fdriver.Vault, envelopeService fdriver.EnvelopeService, ledger fdriver.Ledger, rwsetLoaderService fdriver.RWSetLoader, eventsPublisher events.Publisher, channelMembershipService fdriver.MembershipService, fabricFinality fcommitter.FabricFinality, dependencyResolver fcommitter.DependencyResolver, quiet bool, listenerManager fdriver.ListenerManager, tracerProvider trace.TracerProvider, metricsProvider metrics.Provider) (*fcommitter.Committer, error) {
	// we register the BFT broadcaster for arma consensusType
	os, ok := nw.OrderingService().(*ordering.Service)
	if !ok {
		return nil, errors.New("ordering service is not a committer.OrderingService")
	}
	os.Broadcasters["arma"] = os.Broadcasters[ordering.BFT]

	c := fcommitter.New(
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
	)

	// consider meta namespace transactions to be stored in the vault
	if err := c.ProcessNamespace(types.MetaNamespaceID); err != nil {
		return nil, err
	}

	// register fabricx transaction handler
	committer2.RegisterTransactionHandler(c)
	return c, nil
}
