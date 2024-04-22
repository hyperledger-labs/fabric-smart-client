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
	"github.com/pkg/errors"
)

var (
	logger = flogging.MustGetLogger("orion-sdk.core")
	key    = reflect.TypeOf((*driver.OrionNetworkServiceProvider)(nil))
)

type ONSProvider struct {
	sp     view.ServiceProvider
	config *Config
	ctx    context.Context

	networksMutex sync.Mutex
	networks      map[string]driver.OrionNetworkService
}

func NewOrionNetworkServiceProvider(sp view.ServiceProvider, config *Config) (*ONSProvider, error) {
	provider := &ONSProvider{
		sp:       sp,
		config:   config,
		networks: map[string]driver.OrionNetworkService{},
	}
	return provider, nil
}

func (p *ONSProvider) Start(ctx context.Context) error {
	p.ctx = ctx
	for _, name := range p.config.Names() {
		fns, err := p.OrionNetworkService(name)
		if err != nil {
			return errors.Wrapf(err, "failed to start orion network service [%s]", name)
		}
		if err := fns.Committer().Start(ctx); err != nil {
			return errors.WithMessagef(err, "failed to start committer on orion network service [%s]", name)
		}
		if err := fns.DeliveryService().StartDelivery(ctx); err != nil {
			return errors.WithMessagef(err, "failed to start delivery on orion network service [%s]", name)
		}
	}
	return nil
}

func (p *ONSProvider) Stop() error {
	for _, name := range p.config.Names() {
		fns, err := p.OrionNetworkService(name)
		if err != nil {
			return errors.Wrapf(err, "failed to start orion network service [%s]", name)
		}
		fns.DeliveryService().Stop()
	}
	return nil
}

func (p *ONSProvider) Names() []string {
	return p.config.Names()
}

func (p *ONSProvider) DefaultName() string {
	return p.config.DefaultName()
}

func (p *ONSProvider) OrionNetworkService(network string) (driver.OrionNetworkService, error) {
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
			logger.Errorf("Failed to create new network service for [%s]: [%s]", network, err)
			return nil, err
		}
		p.networks[network] = net
	}
	return net, nil
}

func (p *ONSProvider) newONS(network string) (driver.OrionNetworkService, error) {
	c, err := config.New(view.GetConfigService(p.sp), network, network == p.config.defaultName)
	if err != nil {
		return nil, err
	}

	return generic.NewNetwork(p.ctx, p.sp, c, network)
}

func GetOrionNetworkServiceProvider(sp view.ServiceProvider) driver.OrionNetworkServiceProvider {
	s, err := sp.GetService(key)
	if err != nil {
		logger.Errorf("Failed to get service [%s]: [%s]", key, err)
		return nil
	}
	return s.(driver.OrionNetworkServiceProvider)
}
