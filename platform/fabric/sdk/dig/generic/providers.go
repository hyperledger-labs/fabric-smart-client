/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	committer2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	gdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ledger"
	mspdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	vdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
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
	ConfigProvider      config.Provider
	MetricsProvider     metrics.Provider
	EndpointService     vdriver.EndpointService
	SigService          *sig.Service
	DeserializerManager mspdriver.DeserializerManager
	IdProvider          vdriver.IdentityProvider
	KVS                 *kvs.KVS
	ChannelProvider     generic.ChannelProvider       `name:"generic-channel-provider"`
	IdentityLoaders     []gdriver.NamedIdentityLoader `group:"identity-loaders"`
}) core.NamedDriver {
	d := core.NamedDriver{
		Name: config2.GenericDriver,
		Driver: gdriver.NewProvider(
			in.ConfigProvider,
			in.MetricsProvider,
			in.EndpointService,
			in.SigService,
			in.ChannelProvider,
			in.DeserializerManager,
			in.IdProvider,
			in.IdentityLoaders,
			in.KVS,
		),
	}
	return d
}

func NewChannelProvider(in struct {
	dig.In
	ConfigProvider  config.Provider
	KVS             *kvs.KVS
	Publisher       events.Publisher
	Hasher          hash.Hasher
	TracerProvider  trace.TracerProvider
	Drivers         []dbdriver.NamedDriver `group:"db-drivers"`
	MetricsProvider metrics.Provider
}) generic.ChannelProvider {
	return generic.NewChannelProvider(
		in.KVS,
		in.Publisher,
		in.Hasher,
		in.TracerProvider,
		in.MetricsProvider,
		in.Drivers,
		vault.New,
		generic.NewChannelConfigProvider(in.ConfigProvider),
		committer2.NewFinalityListenerManagerProvider[driver.ValidationCode](in.TracerProvider),
		committer.NewSerialDependencyResolver(),
		func(channelName string, chaincodeManager driver.ChaincodeManager, localMembership driver.LocalMembership, configService driver.ConfigService, transactionManager driver.TransactionManager) driver.Ledger {
			return ledger.New(channelName, chaincodeManager, localMembership, configService, transactionManager)
		},
		func(network string, channel string, envelopeService driver.EnvelopeService, transactionService driver.EndorserTransactionService, transactionManager driver.TransactionManager, vault driver.RWSetInspector) driver.RWSetLoader {
			return rwset.NewLoader(network, channel, envelopeService, transactionService, transactionManager, vault)
		},
		committer.New,
		true,
	)
}
