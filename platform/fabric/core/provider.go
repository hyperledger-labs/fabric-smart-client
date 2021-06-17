/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package core

import (
	"reflect"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server"
)

type fnsProvider struct {
	sp       view.ServiceProvider
	networks map[string]driver.FabricNetworkService
}

func NewFabricNetworkServiceProvider(sp view.ServiceProvider) (*fnsProvider, error) {
	provider := &fnsProvider{
		sp:       sp,
		networks: map[string]driver.FabricNetworkService{},
	}
	return provider, nil
}

func (m *fnsProvider) FabricNetworkService(network string) (driver.FabricNetworkService, error) {
	if len(network) == 0 {
		network = "default"
	}

	net, ok := m.networks[network]
	if !ok {
		var err error
		net, err = m.newFNS(network)
		if err != nil {
			return nil, err
		}
		m.networks[network] = net
	}
	return net, nil
}

func (m *fnsProvider) newFNS(network string) (driver.FabricNetworkService, error) {
	// bridge services
	config := generic.NewConfig(view.GetConfigService(m.sp))
	sigService := generic.NewSigService(m.sp)

	// Endpoint service
	resolverService, err := endpoint.NewResolverService(
		view.GetConfigService(m.sp),
		view.GetEndpointService(m.sp),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed instantiating fabric endpoint resolver")
	}
	if err := resolverService.LoadResolvers(); err != nil {
		return nil, errors.Wrap(err, "failed loading fabric endpoint resolvers")
	}
	endpointService, err := generic.NewEndpointResolver(resolverService, view.GetEndpointService(m.sp))
	if err != nil {
		return nil, errors.Wrap(err, "failed loading endpoint service")
	}

	// Local MSP Manager
	mspService := msp.NewLocalMSPManager(m.sp, config, sigService, view.GetEndpointService(m.sp), view.GetIdentityProvider(m.sp).DefaultIdentity())
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
		m.sp,
		network,
		config,
		idProvider,
		mspService,
		sigService,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed instantiating fabric service provider")
	}

	finality.InstallHandler(server.GetServer(m.sp), net)

	return net, nil
}

func GetFabricNetworkServiceProvider(sp view.ServiceProvider) driver.FabricNetworkServiceProvider {
	s, err := sp.GetService(reflect.TypeOf((*driver.FabricNetworkServiceProvider)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(driver.FabricNetworkServiceProvider)
}
