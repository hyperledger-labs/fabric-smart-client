/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package core

import (
	"reflect"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server"
)

type fnsProvider struct {
	sp       view.ServiceProvider
	networks map[string]api.FabricNetworkService
}

func NewFabricNetworkServiceProvider(sp view.ServiceProvider) (*fnsProvider, error) {
	provider := &fnsProvider{
		sp:       sp,
		networks: map[string]api.FabricNetworkService{},
	}
	return provider, nil
}

func (m *fnsProvider) FabricNetworkService(network string) (api.FabricNetworkService, error) {
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

func (m *fnsProvider) newFNS(network string) (api.FabricNetworkService, error) {
	config := generic.NewConfig(view.GetConfigService(m.sp))
	sigService := generic.NewSigService(m.sp)

	mspService := msp.NewLocalMSPManager(m.sp, config, sigService, view.GetEndpointService(m.sp))
	if err := mspService.Load(); err != nil {
		return nil, err
	}

	me, _ := mspService.GetDefaultIdentity()
	idProvider, err := id.NewProvider(m.sp, me)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating id provider")
	}

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

func GetFabricNetworkServiceProvider(sp view.ServiceProvider) api.FabricNetworkServiceProvider {
	s, err := sp.GetService(reflect.TypeOf((*api.FabricNetworkServiceProvider)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(api.FabricNetworkServiceProvider)
}
