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

	"github.com/hyperledger/fabric-lib-go/common/flogging"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

var (
	fabricNetworkServiceType = reflect.TypeFor[*driver.FabricNetworkServiceProvider]()
	logger                   = logging.MustGetLogger()
)

// NamedDriver associates a driver.Driver implementation with the name a network's
// configuration (FSNConfig.Driver) can reference to select it.
type NamedDriver struct {
	Name string
	driver.Driver
}

// FSNProvider is the driver.FabricNetworkServiceProvider implementation: it lazily builds and
// caches, one per network name, the driver.FabricNetworkService for every Fabric network known
// to its Config, and supports adding further networks at runtime via AddNetwork.
type FSNProvider struct {
	config *Config

	networksMutex sync.Mutex
	configService DynamicConfigService
	networks      map[string]driver.FabricNetworkService
	drivers       map[string]driver.Driver
	validators    []NetworkConfigValidator
}

// NewFabricNetworkServiceProvider builds an FSNProvider from configService (scanned once, at
// construction time, to seed its Config), the set of named drivers available to instantiate
// each configured network, and the NetworkConfigValidators (if any) that every subsequent
// AddNetwork call on the returned provider must satisfy.
func NewFabricNetworkServiceProvider(configService DynamicConfigService, namedDrivers []NamedDriver, validators []NetworkConfigValidator) (*FSNProvider, error) {
	fnsConfig, err := NewConfig(configService)
	if err != nil {
		return nil, err
	}
	drivers := map[string]driver.Driver{}
	for _, d := range namedDrivers {
		drivers[d.Name] = d
	}
	provider := &FSNProvider{
		config:        fnsConfig,
		configService: configService,
		networks:      map[string]driver.FabricNetworkService{},
		drivers:       drivers,
		validators:    validators,
	}
	provider.InitFabricLogging()
	return provider, nil
}

// AddNetwork validates the given raw yaml configuration and, if it describes one or more new
// Fabric networks, merges it into the live configuration tree, making the new network(s)
// available to subsequent calls to FabricNetworkService. It does not start any committer or
// delivery service for the new network(s): those are (lazily) created and started the first
// time FabricNetworkService is called for the new network name.
//
// The configuration is checked against the built-in Fabric naming rules for networks and
// channels, and against every NetworkConfigValidator this provider was configured with.
func (p *FSNProvider) AddNetwork(raw []byte) error {
	return p.config.AddNetwork(raw, p.validators...)
}

// Start instantiates every configured Fabric network and starts the committer and delivery
// service of each of its channels. It does not start any network added afterwards via
// AddNetwork; those are instead lazily started via FabricNetworkService/newFNS materialization
// on first access, without their committer/delivery being started automatically.
func (p *FSNProvider) Start(ctx context.Context) error {
	// What's the default network?
	// TODO: add listener to fabric service when a channel is opened.
	for _, name := range p.config.Names() {
		fns, err := p.FabricNetworkService(name)
		if err != nil {
			return errors.Wrapf(err, "failed to start fabric network service [%s]", name)
		}
		for _, channelName := range fns.ConfigService().ChannelIDs() {
			ch, err := fns.Channel(channelName)
			if err != nil {
				return errors.Wrapf(err, "failed to get channel [%s] for fabric network service [%s]", channelName, name)
			}
			logger.Debugf("start fabric [%s:%s]'s commit service...", name, channelName)
			if err := ch.Committer().Start(ctx); err != nil {
				return errors.WithMessagef(err, "failed to start committer on channel [%s] for fabric network service [%s]", channelName, name)
			}
			logger.Debugf("start fabric [%s:%s]'s delivery service...", name, channelName)
			if err := ch.Delivery().Start(ctx); err != nil {
				return errors.WithMessagef(err, "failed to start delivery on channel [%s] for fabric network service [%s]", channelName, name)
			}
		}
	}

	return nil
}

// Stop closes every channel of every Fabric network that has been instantiated so far (i.e.
// every network for which FabricNetworkService has been called at least once).
func (p *FSNProvider) Stop() error {
	for _, networkName := range p.config.Names() {
		fns, err := p.FabricNetworkService(networkName)
		if err != nil {
			return err
		}
		for _, channelName := range fns.ConfigService().ChannelIDs() {
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

// Names returns the names of every currently configured Fabric network, including any added
// at runtime via AddNetwork.
func (p *FSNProvider) Names() []string {
	return p.config.Names()
}

// DefaultName returns the name of the default Fabric network.
func (p *FSNProvider) DefaultName() string {
	return p.config.DefaultName()
}

// FabricNetworkService returns the driver.FabricNetworkService for the given network name,
// instantiating and caching it on first access. If network is empty, the default network is
// used instead.
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

// InitFabricLogging initializes the fabric logging system
// using the FSC configuration.
func (p *FSNProvider) InitFabricLogging() {
	flogging.Init(flogging.Config{
		Format:  p.configService.GetString("logging.format"),
		Writer:  os.Stderr,
		LogSpec: p.configService.GetString("logging.spec"),
	})
}

// newFNS instantiates the driver.FabricNetworkService for network by re-reading the live
// configuration (so that a network added at runtime via AddNetwork is picked up) and
// delegating to the driver named by its FSNConfig.Driver, or to every registered driver in
// turn if none is specified.
func (p *FSNProvider) newFNS(network string) (driver.FabricNetworkService, error) {
	fnsConfig, err := NewConfig(p.configService)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to load configuration")
	}

	netConfig, err := fnsConfig.Config(network)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to get configuration for [%s]", network)
	}
	if len(netConfig.Driver) != 0 {
		logger.Debugf("instantiate Fabric Network Service [%s] with driver [%s]", network, netConfig.Driver)
		// use the suggested driver
		driver, ok := p.drivers[netConfig.Driver]
		if !ok {
			return nil, errors.Errorf("driver [%s] is not registered", netConfig.Driver)
		}
		nw, err := driver.New(network, network == p.config.defaultName)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create network [%s]", network)
		}
		return nw, nil
	}

	logger.Debugf("no driver specified for network [%s], try all", network)

	// try all available drivers
	for _, d := range p.drivers {
		nw, err := d.New(network, network == p.config.defaultName)
		if err != nil {
			logger.Warningf("failed to create network [%s]: %s", network, err)
			continue
		}
		if nw != nil {
			return nw, nil
		}
	}
	return nil, errors.Errorf("no network driver found for [%s]", network)
}

// GetFabricNetworkServiceProvider retrieves the driver.FabricNetworkServiceProvider registered
// with the given services.Provider.
func GetFabricNetworkServiceProvider(sp services.Provider) (driver.FabricNetworkServiceProvider, error) {
	s, err := sp.GetService(fabricNetworkServiceType)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting fabric network service provider")
	}
	return s.(driver.FabricNetworkServiceProvider), nil
}
