/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"context"
	"errors"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/dig"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	commondig "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/identity"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channel"
	committerconfig "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	committergrpc "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/membership"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
)

func Install(sdk commondig.SDK) error {
	return errors.Join(
		sdk.Container().Provide(committerconfig.NewProvider, dig.As(
			new(committergrpc.ServiceConfigProvider),
			new(finality.ServiceConfigProvider),
			new(queryservice.ServiceConfigProvider),
		)),
		sdk.Container().Provide(committergrpc.NewClientProvider, dig.As(
			new(ledger.GRPCClientProvider),
			new(queryservice.GRPCClientProvider),
			new(finality.GRPCClientProvider),
		)),
		sdk.Container().Provide(ledger.NewProvider),
		sdk.Container().Provide(finality.NewListenerManagerProvider),
		sdk.Container().Provide(digutils.Identity[*finality.Provider](), dig.As(new(finality.ListenerManagerProvider))),
		sdk.Container().Provide(queryservice.NewProvider, dig.As(new(queryservice.Provider))),
		sdk.Container().Provide(NewChannelProvider, dig.As(new(generic.ChannelProvider))),
	)
}

func RegisterLegacy(sdk commondig.SDK) error {
	return errors.Join(
		digutils.Register[finality.ListenerManagerProvider](sdk.Container()),
		digutils.Register[queryservice.Provider](sdk.Container()),
		digutils.Register[*ledger.Provider](sdk.Container()),
	)
}

func InitializeProviders(sdk commondig.SDK, ctx context.Context) error {
	return sdk.Container().Invoke(func(in struct {
		dig.In
		FinalityProvider *finality.Provider
		LedgerProvider   *ledger.Provider
	}) error {
		in.FinalityProvider.Initialize(ctx)
		in.LedgerProvider.Initialize(ctx)
		return nil
	})
}

func NewChannelProvider(in struct {
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
	IdProvider              identity.ViewIdentityProvider
	EnvelopeStore           fdriver.EnvelopeStore
	MetadataStore           fdriver.MetadataStore
	EndorseTxStore          fdriver.EndorseTxStore
	Drivers                 multiplexed.Driver
}) generic.ChannelProvider {
	channelConfigProvider := generic.NewChannelConfigProvider(in.ConfigProvider)
	return channel.NewProvider(
		in.ConfigProvider,
		in.EnvelopeStore,
		in.MetadataStore,
		in.EndorseTxStore,
		in.Drivers,
		func(channelName string, configService fdriver.ConfigService, _ driver.VaultStore) (fdriver.Vault, error) {
			return vault.New(configService, channelName, in.QueryServiceProvider)
		},
		channelConfigProvider,
		func(channelName string, nw fdriver.FabricNetworkService, chaincodeManager fdriver.ChaincodeManager) (fdriver.Ledger, error) {
			return in.LedgerProvider.NewLedger(nw.Name(), channelName)
		},
		func(channel string, nw fdriver.FabricNetworkService, envelopeService fdriver.EnvelopeService, transactionService fdriver.EndorserTransactionService, vault fdriver.RWSetInspector) (fdriver.RWSetLoader, error) {
			return NewRWSetLoader(channel, nw, envelopeService, transactionService, vault), nil
		},
		func(
			nw fdriver.FabricNetworkService,
			channel string,
			peerManager delivery.Services,
			ledger fdriver.Ledger,
			vault delivery.Vault,
			callback fdriver.BlockCallback,
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
				[]cb.HeaderType{cb.HeaderType_MESSAGE},
			)
		},
		func(channelName string) fdriver.MembershipService {
			return membership.NewService(channelName)
		},
		false,
		in.QueryServiceProvider,
		in.ListenerManagerProvider,
	)
}

func NewRWSetLoader(channel string, nw fdriver.FabricNetworkService, envelopeService fdriver.EnvelopeService, transactionService fdriver.EndorserTransactionService, vault fdriver.RWSetInspector) fdriver.RWSetLoader {
	return rwset.NewLoader(nw.Name(), channel, envelopeService, transactionService, nw.TransactionManager(), vault)
}
