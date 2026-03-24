/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/identity"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channel"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/membership"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
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
}) core.NamedDriver {
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
	ConfigProvider       config.Provider
	KVS                  *kvs.KVS
	LedgerProvider       *ledger.Provider
	Publisher            events.Publisher
	TracerProvider       trace.TracerProvider
	MetricsProvider      metrics.Provider
	QueryServiceProvider queryservice.Provider
	IdentityLoaders      []identity.NamedIdentityLoader `group:"identity-loaders"`
	EndpointService      identity.EndpointService
	IdProvider           identity.ViewIdentityProvider
	EnvelopeStore        fdriver.EnvelopeStore
	MetadataStore        fdriver.MetadataStore
	EndorseTxStore       fdriver.EndorseTxStore
	Drivers              multiplexed.Driver
}) generic.ChannelProvider {
	channelConfigProvider := generic.NewChannelConfigProvider(in.ConfigProvider)
	return channel.NewProvider(
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
		// delivery service constructor
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
		// membership service
		func(channelName string) fdriver.MembershipService {
			return membership.NewService(channelName)
		},
		false,
		in.QueryServiceProvider,
	)
}

func NewRWSetLoader(channel string, nw fdriver.FabricNetworkService, envelopeService fdriver.EnvelopeService, transactionService fdriver.EndorserTransactionService, vault fdriver.RWSetInspector) fdriver.RWSetLoader {
	return rwset.NewLoader(nw.Name(), channel, envelopeService, transactionService, nw.TransactionManager(), vault)
}
