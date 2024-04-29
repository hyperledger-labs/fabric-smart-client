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

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
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
	configService driver2.ConfigService
	networks      map[string]driver.FabricNetworkService
}

func NewFabricNetworkServiceProvider(sp view.ServiceProvider, configService driver2.ConfigService) (*FSNProvider, error) {
	fnsConfig, err := NewConfig(configService)
	if err != nil {
		return nil, err
	}
	provider := &FSNProvider{
		sp:            sp,
		config:        fnsConfig,
		configService: configService,
		networks:      map[string]driver.FabricNetworkService{},
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
		for _, channelName := range fns.ConfigService().ChannelIDs() {
			ch, err := fns.Channel(channelName)
			if err != nil {
				return errors.Wrapf(err, "failed to get channel [%s] for fabric network service [%s]", channelName, name)
			}
			logger.Infof("start fabric [%s:%s]'s commit service...", name, channelName)
			if err := ch.Committer().Start(ctx); err != nil {
				return errors.WithMessagef(err, "failed to start committer on channel [%s] for fabric network service [%s]", channelName, name)
			}
			logger.Infof("start fabric [%s:%s]'s delivery service...", name, channelName)
			if err := ch.Delivery().Start(ctx); err != nil {
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

// InitFabricLogging initializes the fabric logging system
// using the FSC configuration.
func (p *FSNProvider) InitFabricLogging() {
	// read in the legacy logging level settings and, if set,
	// notify users of the FSCNODE_LOGGING_SPEC env variable
	var loggingLevel string
	if p.configService.GetString("logging_level") != "" {
		loggingLevel = p.configService.GetString("logging_level")
	} else {
		loggingLevel = p.configService.GetString("logging.level")
	}
	if loggingLevel != "" {
		logger.Warning("CORE_LOGGING_LEVEL is no longer supported, please use the FSCNODE_LOGGING_SPEC environment variable")
	}
	loggingSpec := os.Getenv("FSCNODE_LOGGING_SPEC")
	loggingFormat := os.Getenv("FSCNODE_LOGGING_FORMAT")
	if len(loggingSpec) == 0 {
		loggingSpec = p.configService.GetString("logging.spec")
	}
	fabricLogging.Init(fabricLogging.Config{
		Format:  loggingFormat,
		Writer:  os.Stderr,
		LogSpec: loggingSpec,
	})
}

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
		driver, ok := drivers[netConfig.Driver]
		if !ok {
			return nil, errors.Errorf("driver [%s] is not registered", netConfig.Driver)
		}
		nw, err := driver.New(p.sp, network, network == p.config.defaultName)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create network [%s]", network)
		}
		return nw, nil
	}

	logger.Debugf("no driver specified for network [%s], try all", network)

	// try all available drivers
	for _, d := range drivers {
		nw, err := d.New(p.sp, network, network == p.config.defaultName)
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

func GetFabricNetworkServiceProvider(sp view.ServiceProvider) (driver.FabricNetworkServiceProvider, error) {
	s, err := sp.GetService(fabricNetworkServiceType)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting fabric network service provider")
	}
	return s.(driver.FabricNetworkServiceProvider), nil
}
