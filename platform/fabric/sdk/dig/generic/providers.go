/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	gdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	mspdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/sig"
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
	ConfigProvider          config.Provider
	MetricsProvider         metrics.Provider
	EndpointService         vdriver.EndpointService
	SigService              *sig.Service
	DeserializerManager     mspdriver.DeserializerManager
	IdProvider              vdriver.IdentityProvider
	KVS                     *kvs.KVS
	Publisher               events.Publisher
	Hasher                  hash.Hasher
	TracerProvider          trace.TracerProvider
	Drivers                 []dbdriver.NamedDriver `group:"db-drivers"`
	ListenerManagerProvider driver.ListenerManagerProvider
}) core.NamedDriver {
	d := core.NamedDriver{
		Name: "generic",
		Driver: gdriver.NewProvider(
			in.ConfigProvider,
			in.MetricsProvider,
			in.EndpointService,
			in.SigService,
			in.DeserializerManager,
			in.IdProvider,
			in.KVS,
			in.Publisher,
			in.Hasher,
			in.TracerProvider,
			in.Drivers,
			in.ListenerManagerProvider,
		),
	}
	return d
}
