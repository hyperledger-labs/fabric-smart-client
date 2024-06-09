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

func NewChannelProvider(kvss *kvs.KVS, publisher events.Publisher, hasher hash.Hasher, tracerProvider trace.TracerProvider) channelProviderResult {
	return channelProviderResult{Provider: generic.NewProvider(kvss, publisher, hasher, tracerProvider)}
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
	Drivers       []NamedDriver `group:"drivers"`
}) (*core.FSNProvider, error) {
	return core.NewFabricNetworkServiceProvider(nil, in.ConfigService)
}

type NamedDriver struct {
	Name string
	driver.Driver
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
}) NamedDriver {
	d := NamedDriver{
		Name:   "generic",
		Driver: driver4.NewProvider(in.ConfigProvider, in.ChannelProvider, in.IdentityProvider, in.MetricsProvider, in.EndpointService, in.SigService, in.DeserializerManager, in.IdProvider, in.KVS),
	}
	core.Register(d.Name, d.Driver)
	return d
}
