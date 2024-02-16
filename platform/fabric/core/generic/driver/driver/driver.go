/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	metrics2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	metrics3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

var logger = flogging.MustGetLogger("fabric-sdk.core.generic.driver")

type driverProvider struct {
	configProvider   ConfigProvider
	identityProvider IdentityProvider
	metricsProvider  metrics3.Provider
	endpointService  driver2.EndpointService
	channelProvider  generic.ChannelProvider
	sigService       driver.SignerService
	identityLoaders  map[string]driver3.IdentityLoader
}

func NewDriverProvider(configProvider ConfigProvider, channelProvider generic.ChannelProvider, idProvider IdentityProvider, metricsProvider metrics3.Provider, endpointService driver2.EndpointService, sigService driver.SignerService, identityLoaders map[string]driver3.IdentityLoader) *driverProvider {
	return &driverProvider{
		configProvider:   configProvider,
		channelProvider:  channelProvider,
		identityProvider: idProvider,
		metricsProvider:  metricsProvider,
		endpointService:  endpointService,
		sigService:       sigService,
		identityLoaders:  identityLoaders,
	}
}

func (d *driverProvider) New(sp view.ServiceProvider, network string, defaultNetwork bool) (driver.FabricNetworkService, error) {
	logger.Debugf("creating new fabric network service for network [%s]", network)

	idProvider, err := d.identityProvider.New(network)
	if err != nil {
		return nil, err
	}

	// bridge services
	genericConfig, err := d.configProvider.GetConfig(network)
	if err != nil {
		return nil, err
	}

	// Local MSP Manager
	mspService := msp.NewLocalMSPManager(
		sp,
		genericConfig,
		d.sigService,
		d.endpointService,
		view.GetIdentityProvider(sp).DefaultIdentity(),
		genericConfig.MSPCacheSize(),
	)
	for idType, loader := range d.identityLoaders {
		mspService.PutIdentityLoader(idType, loader)
	}
	if err := mspService.Load(); err != nil {
		return nil, fmt.Errorf("failed loading local msp service: %w", err)
	}

	// New Network
	net, err := generic.NewNetwork(
		sp,
		network,
		genericConfig,
		idProvider,
		mspService,
		d.sigService,
		metrics2.NewMetrics(d.metricsProvider),
		d.channelProvider.NewChannel,
	)
	if err != nil {
		return nil, fmt.Errorf("failed instantiating fabric service provider: %w", err)
	}
	if err := net.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize fabric service provider: %w", err)
	}

	return net, nil
}
