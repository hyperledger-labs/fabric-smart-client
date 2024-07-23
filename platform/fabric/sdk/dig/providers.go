/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/config"
	driver4 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/identity"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	driver5 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger/fabric-protos-go/common"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/dig"
)

type channelProviderResult struct {
	dig.Out
	generic.Provider `name:"generic"`
}

func NewChannelProvider(in struct {
	dig.In
	KVS                     *kvs.KVS
	Publisher               events.Publisher
	Hasher                  hash.Hasher
	TracerProvider          trace.TracerProvider
	Drivers                 []driver5.NamedDriver `group:"db-drivers"`
	ChannelConfigProvider   driver.ChannelConfigProvider
	ListenerManagerProvider driver.ListenerManagerProvider
}) channelProviderResult {
	return channelProviderResult{Provider: generic.NewProvider(in.KVS, in.Publisher, in.Hasher, in.TracerProvider, in.Drivers, in.ChannelConfigProvider, in.ListenerManagerProvider)}
}

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

func NewFSNProvider(in struct {
	dig.In
	ConfigService driver2.ConfigService
	Drivers       []core.NamedDriver `group:"drivers"`
}) (*core.FSNProvider, error) {
	return core.NewFabricNetworkServiceProvider(in.ConfigService, in.Drivers)
}

func NewDriver(in struct {
	dig.In
	ChannelProvider     generic.Provider `name:"generic"`
	ConfigProvider      config.Provider
	IdentityProvider    identity.Provider
	MetricsProvider     metrics.Provider
	EndpointService     driver2.EndpointService
	SigService          *sig.Service
	DeserializerManager driver3.DeserializerManager
	IdProvider          driver2.IdentityProvider
	KVS                 *kvs.KVS
}) core.NamedDriver {
	d := core.NamedDriver{
		Name:   "generic",
		Driver: driver4.NewProvider(in.ConfigProvider, in.ChannelProvider, in.IdentityProvider, in.MetricsProvider, in.EndpointService, in.SigService, in.DeserializerManager, in.IdProvider, in.KVS),
	}
	return d
}
