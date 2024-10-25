/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/identity"
	gmetrics "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/sig"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	vdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

var logger = flogging.MustGetLogger("fabric-sdk.core.generic.driver")

type Provider struct {
	configProvider     config.Provider
	identityProvider   identity.Provider
	metricsProvider    metrics.Provider
	channelProvider    generic.ChannelProvider
	sigService         *sig.Service
	mspManagerProvider MSPManagerProvider
}

func NewProvider(
	configProvider config.Provider,
	metricsProvider metrics.Provider,
	endpointService identity.EndpointService,
	sigService *sig.Service,
	channelProvider generic.ChannelProvider,
	mspManagerProvider MSPManagerProvider,
) *Provider {
	return &Provider{
		configProvider:     configProvider,
		channelProvider:    channelProvider,
		identityProvider:   identity.NewProvider(configProvider, endpointService),
		metricsProvider:    metricsProvider,
		sigService:         sigService,
		mspManagerProvider: mspManagerProvider,
	}
}

type NamedIdentityLoader struct {
	Name string
	driver.IdentityLoader
}

type MSPManagerProvider interface {
	New(network string) (fdriver.LocalMembership, error)
}

func NewLocalMSPManagerProvider(configProvider config.Provider, endpointService driver.BinderService, sigService *sig.Service, identityLoaders []NamedIdentityLoader, deserializerManager driver.DeserializerManager, idProvider vdriver.IdentityProvider, kvss *kvs.KVS) *localMSPManagerProvider {
	return &localMSPManagerProvider{
		configProvider:      configProvider,
		endpointService:     endpointService,
		sigService:          sigService,
		identityLoaders:     identityLoaders,
		deserializerManager: deserializerManager,
		idProvider:          idProvider,
		kvss:                kvss,
	}
}

type localMSPManagerProvider struct {
	configProvider      config.Provider
	endpointService     driver.BinderService
	sigService          *sig.Service
	identityLoaders     []NamedIdentityLoader
	deserializerManager driver.DeserializerManager
	idProvider          vdriver.IdentityProvider
	kvss                *kvs.KVS
}

func (p *localMSPManagerProvider) New(network string) (fdriver.LocalMembership, error) {
	genericConfig, err := p.configProvider.GetConfig(network)
	if err != nil {
		return nil, err
	}

	// Local MSP Manager
	mspService := msp.NewLocalMSPManager(
		genericConfig.(driver.Config),
		p.kvss,
		p.sigService,
		p.endpointService,
		p.idProvider.DefaultIdentity(),
		p.deserializerManager,
		genericConfig.MSPCacheSize(),
	)
	for _, loader := range p.identityLoaders {
		mspService.PutIdentityLoader(loader.Name, loader.IdentityLoader)
	}
	if err := mspService.Load(); err != nil {
		return nil, fmt.Errorf("failed loading local msp service: %w", err)
	}
	return mspService, nil
}

func (d *Provider) New(network string, _ bool) (fdriver.FabricNetworkService, error) {
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

	mspService, err := d.mspManagerProvider.New(network)
	if err != nil {
		return nil, err
	}

	// New Network
	net, err := generic.NewNetwork(
		network,
		genericConfig,
		idProvider,
		mspService,
		d.sigService,
		gmetrics.NewMetrics(d.metricsProvider),
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
