/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package core

import (
	"context"
	"os"
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/views"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	fabricLogging "github.com/hyperledger/fabric/common/flogging"
	"github.com/pkg/errors"
)

var (
	fabricNetworkServiceType = reflect.TypeOf((*driver.FabricNetworkServiceProvider)(nil))
	logger                   = flogging.MustGetLogger("fabric-sdk.core")
)

type FSNProvider struct {
	sp     view.ServiceProvider
	config *Config

	networksMutex sync.Mutex
	networks      map[string]driver.FabricNetworkService
}

func NewFabricNetworkServiceProvider(sp view.ServiceProvider, config *Config) (*FSNProvider, error) {
	provider := &FSNProvider{
		sp:       sp,
		config:   config,
		networks: map[string]driver.FabricNetworkService{},
	}
	if err := provider.InstallViews(); err != nil {
		return nil, errors.WithMessage(err, "failed to install fns provider")
	}
	provider.InitFabricLogging()
	return provider, nil
}

func (p *FSNProvider) Start(ctx context.Context) error {
	// What's the default network?
	// TODO: add listener to fabric service when a channel is opened.
	for _, name := range p.config.Names() {
		fns, err := p.FabricNetworkService(name)
		if err != nil {
			return errors.Wrapf(err, "failed to start fabric network service [%s]", name)
		}
		for _, channelName := range fns.Channels() {
			ch, err := fns.Channel(channelName)
			if err != nil {
				return errors.Wrapf(err, "failed to get channel [%s] for fabric network service [%s]", channelName, name)
			}
			logger.Infof("start fabric [%s:%s]'s delivery service...", name, channelName)
			if err := ch.StartDelivery(ctx); err != nil {
				return errors.WithMessagef(err, "failed to start delivery on channel [%s] for fabric network service [%s]", channelName, name)
			}
		}
	}

	return nil
}

func (p *FSNProvider) Stop() error {
	for _, networkName := range p.config.Names() {
		fns, err := p.FabricNetworkService(networkName)
		if err != nil {
			return err
		}
		for _, channelName := range fns.Channels() {
			ch, err := fns.Channel(channelName)
			if err != nil {
				return err
			}
			if err := ch.Close(); err != nil {
				logger.Errorf("failed closing channel [%s:%s]: [%s]", networkName, channelName, err)
			}
		}
	}
	return nil
}

func (p *FSNProvider) Names() []string {
	return p.config.Names()
}

func (p *FSNProvider) DefaultName() string {
	return p.config.DefaultName()
}

func (p *FSNProvider) FabricNetworkService(network string) (driver.FabricNetworkService, error) {
	p.networksMutex.Lock()
	defer p.networksMutex.Unlock()

	if len(network) == 0 {
		network = p.config.DefaultName()
	}

	net, ok := p.networks[network]
	if !ok {
		var err error
		net, err = p.newFNS(network)
		if err != nil {
			return nil, err
		}
		p.networks[network] = net
	}
	return net, nil
}

func (p *FSNProvider) InstallViews() error {
	if err := view.GetRegistry(p.sp).RegisterResponder(views.NewIsFinalResponderView(p), &finality.IsFinalInitiatorView{}); err != nil {
		return errors.WithMessagef(err, "failed to register finality responder")
	}
	return nil
}

// InitFabricLogging initializes the fabric logging system
// using the FSC configuration.
func (p *FSNProvider) InitFabricLogging() {
	cs := view.GetConfigService(p.sp)
	// read in the legacy logging level settings and, if set,
	// notify users of the FSCNODE_LOGGING_SPEC env variable
	var loggingLevel string
	if cs.GetString("logging_level") != "" {
		loggingLevel = cs.GetString("logging_level")
	} else {
		loggingLevel = cs.GetString("logging.level")
	}
	if loggingLevel != "" {
		logger.Warning("CORE_LOGGING_LEVEL is no longer supported, please use the FSCNODE_LOGGING_SPEC environment variable")
	}
	loggingSpec := os.Getenv("FSCNODE_LOGGING_SPEC")
	loggingFormat := os.Getenv("FSCNODE_LOGGING_FORMAT")
	if len(loggingSpec) == 0 {
		loggingSpec = cs.GetString("logging.spec")
	}
	fabricLogging.Init(fabricLogging.Config{
		Format:  loggingFormat,
		Writer:  os.Stderr,
		LogSpec: loggingSpec,
	})
}

func (p *FSNProvider) newFNS(network string) (driver.FabricNetworkService, error) {
	logger.Debugf("creating new fabric network service for network [%s]", network)
	// bridge services
	c, err := config.New(view.GetConfigService(p.sp), network, network == p.config.defaultName)
	if err != nil {
		return nil, err
	}
	sigService := generic.NewSigService(p.sp)

	// Endpoint service
	resolverService, err := endpoint.NewResolverService(
		c,
		view.GetEndpointService(p.sp),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed instantiating fabric endpoint resolver")
	}
	if err := resolverService.LoadResolvers(); err != nil {
		return nil, errors.Wrap(err, "failed loading fabric endpoint resolvers")
	}
	endpointService, err := generic.NewEndpointResolver(resolverService, view.GetEndpointService(p.sp))
	if err != nil {
		return nil, errors.Wrap(err, "failed loading endpoint service")
	}

	// Local MSP Manager
	mspService := msp.NewLocalMSPManager(
		p.sp,
		c,
		sigService,
		view.GetEndpointService(p.sp),
		view.GetIdentityProvider(p.sp).DefaultIdentity(),
		c.MSPCacheSize(),
	)
	if err := mspService.Load(); err != nil {
		return nil, errors.Wrap(err, "failed loading local msp service")
	}

	// Identity Manager
	idProvider, err := id.NewProvider(endpointService)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating id provider")
	}

	// New Network
	net, err := generic.NewNetwork(
		p.sp,
		network,
		c,
		idProvider,
		mspService,
		sigService,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed instantiating fabric service provider")
	}

	return net, nil
}

func GetFabricNetworkServiceProvider(sp view.ServiceProvider) driver.FabricNetworkServiceProvider {
	s, err := sp.GetService(fabricNetworkServiceType)
	if err != nil {
		logger.Warnf("failed getting fabric network service provider: %s", err)
		return nil
	}
	return s.(driver.FabricNetworkServiceProvider)
}
