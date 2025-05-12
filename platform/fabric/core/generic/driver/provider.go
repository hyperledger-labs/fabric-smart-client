/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/identity"
	gmetrics "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	vdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/trace"
)

var logger = logging.MustGetLogger()

type Provider struct {
	configProvider     config.Provider
	identityProvider   identity.Provider
	metricsProvider    metrics.Provider
	channelProvider    generic.ChannelProvider
	sigService         *sig.Service
	mspManagerProvider identity.MSPManagerProvider
}

func NewProvider(
	configProvider config.Provider,
	metricsProvider metrics.Provider,
	endpointService identity.EndpointService,
	channelProvider generic.ChannelProvider,
	idProvider vdriver.IdentityProvider,
	identityLoaders []identity.NamedIdentityLoader,
	signerKVS driver.SignerInfoStore,
	auditInfoKVS driver.AuditInfoStore,
	kvss *kvs.KVS,
	tracerProvider trace.TracerProvider,
) *Provider {
	deserializerManager := sig.NewMultiplexDeserializer()
	sigService := sig.NewService(deserializerManager, auditInfoKVS, signerKVS)
	return &Provider{
		configProvider:   configProvider,
		channelProvider:  channelProvider,
		identityProvider: identity.NewProvider(configProvider, endpointService),
		metricsProvider:  metricsProvider,
		sigService:       sigService,
		mspManagerProvider: identity.NewMSPManagerProvider(
			configProvider,
			endpointService,
			sigService,
			identityLoaders,
			deserializerManager,
			idProvider,
			kvss,
		),
	}
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
