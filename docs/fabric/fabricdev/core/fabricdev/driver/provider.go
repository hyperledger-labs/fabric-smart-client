/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/docs/fabric/fabricdev/core/fabricdev"
	"github.com/hyperledger-labs/fabric-smart-client/docs/fabric/fabricdev/core/fabricdev/transaction"
	vault3 "github.com/hyperledger-labs/fabric-smart-client/docs/fabric/fabricdev/core/fabricdev/vault"
	committer2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/identity"
	gmetrics "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	vdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger/fabric-protos-go/common"
	"go.opentelemetry.io/otel/trace"
)

var logger = logging.MustGetLogger()

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
	envelopeKVS fdriver.EnvelopeStore,
	metadataKVS fdriver.MetadataStore,
	endorseTxKVS fdriver.EndorseTxStore,
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
	Drivers multiplexed.Driver,
) *Provider {
	return &Provider{
		configProvider: configProvider,
		channelProvider: fabricdev.NewChannelProvider(
			configProvider,
			envelopeKVS,
			metadataKVS,
			endorseTxKVS,
			publisher,
			hasher,
			tracerProvider,
			metricsProvider,
			Drivers,
			func(_ string, configService fdriver.ConfigService, vaultStore cdriver.VaultStore) (*vault2.Vault, error) {
				cachedVault := vault.NewCachedVault(vaultStore, configService.VaultTXStoreCacheSize())
				return vault3.NewVault(cachedVault, metricsProvider, tracerProvider), nil
			},
			generic.NewChannelConfigProvider(configProvider),
			committer2.NewFinalityListenerManagerProvider[fdriver.ValidationCode](tracerProvider),
			committer.NewSerialDependencyResolver(),
			[]common.HeaderType{common.HeaderType_ENDORSER_TRANSACTION},
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

	net.SetTransactionManager(transaction.NewManager())

	return net, nil
}
