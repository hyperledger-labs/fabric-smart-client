/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"go.uber.org/dig"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	commondig "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/identity"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
)

func Install(sdk commondig.SDK) error {
	return sdk.Container().Provide(NewDriver, dig.Group("fabric-platform-drivers"))
}

func NewDriver(in struct {
	dig.In
	ConfigProvider  config.Provider
	MetricsProvider metrics.Provider
	EndpointService identity.EndpointService
	IdProvider      identity.ViewIdentityProvider
	KVS             *kvs.KVS
	SignerInfoStore driver.SignerInfoStore
	AuditInfoStore  driver.AuditInfoStore
	ChannelProvider generic.ChannelProvider
	IdentityLoaders []identity.NamedIdentityLoader `group:"identity-loaders"`
	Drivers         multiplexed.Driver
}) core.NamedDriver {
	return core.NamedDriver{
		Name: fabricx.DriverName,
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
}
