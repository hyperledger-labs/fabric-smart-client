/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"fmt"

	committer2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ledger"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/identity"
	gmetrics "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	vdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/trace"
)

var logger = flogging.MustGetLogger("fabric-sdk.core.generic.driver")

type Provider struct {
	configProvider      config.Provider
	identityProvider    identity.Provider
	metricsProvider     metrics.Provider
	endpointService     driver.BinderService
	channelProvider     generic.ChannelProvider
	sigService          *sig.Service
	identityLoaders     map[string]driver.IdentityLoader
	deserializerManager driver.DeserializerManager
	idProvider          vdriver.IdentityProvider
	kvss                *kvs.KVS
}

func NewProvider(
	configProvider config.Provider,
	metricsProvider metrics.Provider,
	endpointService identity.EndpointService,
	sigService *sig.Service,
	deserializerManager driver.DeserializerManager,
	idProvider vdriver.IdentityProvider,
	kvss *kvs.KVS,
	publisher events.Publisher,
	hasher hash.Hasher,
	tracerProvider trace.TracerProvider,
	Drivers []dbdriver.NamedDriver,
) *Provider {
	return &Provider{
		configProvider: configProvider,
		channelProvider: generic.NewChannelProvider(
			kvss,
			publisher,
			hasher,
			tracerProvider,
			metricsProvider,
			Drivers,
			vault.New,
			generic.NewChannelConfigProvider(configProvider),
			committer2.NewFinalityListenerManagerProvider[fdriver.ValidationCode](tracerProvider),
			committer.NewSerialDependencyResolver(),
			ledger.New,
			rwset.NewLoader,
			committer.New,
		),
		identityProvider:    identity.NewProvider(configProvider, endpointService),
		metricsProvider:     metricsProvider,
		endpointService:     endpointService,
		sigService:          sigService,
		identityLoaders:     map[string]driver.IdentityLoader{},
		deserializerManager: deserializerManager,
		idProvider:          idProvider,
		kvss:                kvss,
	}
}

func (d *Provider) RegisterIdentityLoader(typ string, loader driver.IdentityLoader) {
	d.identityLoaders[typ] = loader
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

	// Local MSP Manager
	mspService := msp.NewLocalMSPManager(
		genericConfig,
		d.kvss,
		d.sigService,
		d.endpointService,
		d.idProvider.DefaultIdentity(),
		d.deserializerManager,
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
