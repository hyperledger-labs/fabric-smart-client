/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package core

import (
	"context"
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
	"github.com/pkg/errors"
)

var (
	index  = reflect.TypeOf((*driver.FabricNetworkServiceProvider)(nil))
	logger = flogging.MustGetLogger("fabric-sdk.core")
)

type fnsProvider struct {
	sp     view.ServiceProvider
	config *Config
	ctx    context.Context

	networksMutex sync.Mutex
	networks      map[string]driver.FabricNetworkService
}

func NewFabricNetworkServiceProvider(sp view.ServiceProvider, config *Config) (*fnsProvider, error) {
	provider := &fnsProvider{
		sp:       sp,
		config:   config,
		networks: map[string]driver.FabricNetworkService{},
	}
	if err := provider.InstallViews(); err != nil {
		return nil, errors.WithMessage(err, "failed to install fns provider")
	}
	return provider, nil
}

func (p *fnsProvider) Start(ctx context.Context) error {
	p.ctx = ctx

	// What's the default network?
	// TODO: add listener to fabric service when a channel is opened.
	for _, name := range p.config.Names() {
		fns, err := p.FabricNetworkService(name)
		if err != nil {
			return err
		}
		for _, ch := range fns.Channels() {
			_, err := fns.Channel(ch)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *fnsProvider) Stop() error {
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

func (p *fnsProvider) Names() []string {
	return p.config.Names()
}

func (p *fnsProvider) DefaultName() string {
	return p.config.DefaultName()
}

func (p *fnsProvider) FabricNetworkService(network string) (driver.FabricNetworkService, error) {
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

func (p *fnsProvider) InstallViews() error {
	view.GetRegistry(p.sp).RegisterResponder(views.NewIsFinalResponderView(p), &finality.IsFinalInitiatorView{})
	return nil
}

func (p *fnsProvider) newFNS(network string) (driver.FabricNetworkService, error) {
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
		p.ctx,
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
	s, err := sp.GetService(index)
	if err != nil {
		logger.Warnf("failed getting fabric network service provider: %s", err)
		return nil
	}
	return s.(driver.FabricNetworkServiceProvider)
}
