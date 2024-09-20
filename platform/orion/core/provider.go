/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package core

import (
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

var (
	logger = flogging.MustGetLogger("orion-sdk.core")
)

type ONSProvider struct {
	configService driver2.ConfigService
	config        *Config
	ctx           context.Context
	kvss          *kvs.KVS
	publisher     events.Publisher
	subscriber    events.Subscriber

	networksMutex           sync.Mutex
	networks                map[string]driver.OrionNetworkService
	drivers                 []driver3.NamedDriver
	metricsProvider         metrics.Provider
	tracerProvider          trace.TracerProvider
	networkConfigProvider   driver.NetworkConfigProvider
	listenerManagerProvider driver.ListenerManagerProvider
}

func NewOrionNetworkServiceProvider(configService driver2.ConfigService, config *Config, kvss *kvs.KVS, publisher events.Publisher, subscriber events.Subscriber, metricsProvider metrics.Provider, tracerProvider trace.TracerProvider, drivers []driver3.NamedDriver, networkConfigProvider driver.NetworkConfigProvider, listenerManagerProvider driver.ListenerManagerProvider) (*ONSProvider, error) {
	provider := &ONSProvider{
		configService:           configService,
		config:                  config,
		kvss:                    kvss,
		publisher:               publisher,
		subscriber:              subscriber,
		networks:                map[string]driver.OrionNetworkService{},
		drivers:                 drivers,
		metricsProvider:         metricsProvider,
		tracerProvider:          tracerProvider,
		networkConfigProvider:   networkConfigProvider,
		listenerManagerProvider: listenerManagerProvider,
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
	c, err := config.New(p.configService, network, network == p.config.defaultName)
	if err != nil {
		return nil, err
	}

	networkConfig, err := p.networkConfigProvider.GetNetworkConfig(network)
	if err != nil {
		return nil, err
	}

	return generic.NewNetwork(p.ctx, p.kvss, p.publisher, p.subscriber, p.metricsProvider, p.tracerProvider, c, network, p.drivers, networkConfig, p.listenerManagerProvider.NewManager())
}
