/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricdev

import (
	fabricdev2 "github.com/hyperledger-labs/fabric-smart-client/docs/fabric/fabricdev/core/fabricdev"
	driver6 "github.com/hyperledger-labs/fabric-smart-client/docs/fabric/fabricdev/core/fabricdev/driver"
	"github.com/hyperledger-labs/fabric-smart-client/docs/fabric/fabricdev/core/fabricdev/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/identity"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	driver5 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/dig"
)

func NewDriver(in struct {
	dig.In
	ConfigProvider          config.Provider
	MetricsProvider         metrics.Provider
	EndpointService         driver2.EndpointService
	SigService              *sig.Service
	DeserializerManager     driver3.DeserializerManager
	IdProvider              driver2.IdentityProvider
	KVS                     *kvs.KVS
	Publisher               events.Publisher
	Hasher                  hash.Hasher
	TracerProvider          trace.TracerProvider
	Drivers                 []driver5.NamedDriver `group:"db-drivers"`
	ListenerManagerProvider driver.ListenerManagerProvider
}) core.NamedDriver {
	d := core.NamedDriver{
		Name: "fabricdev",
		Driver: driver6.NewProvider(
			in.ConfigProvider,
			fabricdev2.NewChannelProvider(
				in.KVS,
				in.Publisher,
				in.Hasher,
				in.TracerProvider,
				in.Drivers,
				vault.New,
				generic.NewChannelConfigProvider(in.ConfigProvider),
				in.ListenerManagerProvider,
			),
			identity.NewProvider(in.ConfigProvider, in.EndpointService),
			in.MetricsProvider,
			in.EndpointService,
			in.SigService,
			in.DeserializerManager,
			in.IdProvider,
			in.KVS,
		),
	}
	return d
}
