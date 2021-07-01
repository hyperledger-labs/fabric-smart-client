/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package core

import (
	"context"
	"reflect"
	"sync"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	view3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view"
)

var logger = flogging.MustGetLogger("fabric-sdk.core")

type fnsProvider struct {
	sp     view.ServiceProvider
	config *Config

	networksMutex sync.Mutex
	networks      map[string]driver.FabricNetworkService
}

func NewFabricNetworkServiceProvider(sp view.ServiceProvider, config *Config) (*fnsProvider, error) {
	provider := &fnsProvider{
		sp:       sp,
		config:   config,
		networks: map[string]driver.FabricNetworkService{},
	}
	return provider, nil
}

func (p *fnsProvider) Start(ctx context.Context) error {
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

func (p *fnsProvider) newFNS(network string) (driver.FabricNetworkService, error) {
	// bridge services
	config, err := generic.NewConfig(view.GetConfigService(p.sp), network, network == p.config.defaultName)
	if err != nil {
		return nil, err
	}
	sigService := generic.NewSigService(p.sp)

	// Endpoint service
	resolverService, err := endpoint.NewResolverService(
		config,
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
	mspService := msp.NewLocalMSPManager(p.sp, config, sigService, view.GetEndpointService(p.sp), view.GetIdentityProvider(p.sp).DefaultIdentity())
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
		config,
		idProvider,
		mspService,
		sigService,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed instantiating fabric service provider")
	}

	finality.InstallHandler(view3.GetService(p.sp), net)

	return net, nil
}

func GetFabricNetworkServiceProvider(sp view.ServiceProvider) driver.FabricNetworkServiceProvider {
	s, err := sp.GetService(reflect.TypeOf((*driver.FabricNetworkServiceProvider)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(driver.FabricNetworkServiceProvider)
}
