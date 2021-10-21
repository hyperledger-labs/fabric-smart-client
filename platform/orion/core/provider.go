/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package core

import (
	"context"
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("orion-sdk.core")

type onsProvider struct {
	sp     view.ServiceProvider
	config *Config
	ctx    context.Context

	networksMutex sync.Mutex
	networks      map[string]driver.OrionNetworkService
}

func NewOrionNetworkServiceProvider(sp view.ServiceProvider, config *Config) (*onsProvider, error) {
	provider := &onsProvider{
		sp:       sp,
		config:   config,
		networks: map[string]driver.OrionNetworkService{},
	}
	return provider, nil
}

func (p *onsProvider) Start(ctx context.Context) error {
	p.ctx = ctx
	return nil
}

func (p *onsProvider) Stop() error {
	return nil
}

func (p *onsProvider) Names() []string {
	return p.config.Names()
}

func (p *onsProvider) DefaultName() string {
	return p.config.DefaultName()
}

func (p *onsProvider) OrionNetworkService(network string) (driver.OrionNetworkService, error) {
	p.networksMutex.Lock()
	defer p.networksMutex.Unlock()

	if len(network) == 0 {
		network = p.config.DefaultName()
	}

	net, ok := p.networks[network]
	if !ok {
		var err error
		net, err = p.newONS(network)
		if err != nil {
			return nil, err
		}
		p.networks[network] = net
	}
	return net, nil
}

func (p *onsProvider) newONS(network string) (driver.OrionNetworkService, error) {
	config, err := config.New(view.GetConfigService(p.sp), network, network == p.config.defaultName)
	if err != nil {
		return nil, err
	}

	return generic.NewNetwork(p.ctx, p.sp, config, network)
}

func GetOrionNetworkServiceProvider(sp view.ServiceProvider) driver.OrionNetworkServiceProvider {
	s, err := sp.GetService(reflect.TypeOf((*driver.OrionNetworkServiceProvider)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(driver.OrionNetworkServiceProvider)
}
