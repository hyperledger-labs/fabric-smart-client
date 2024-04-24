/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
)

var (
	networkServiceProviderType = reflect.TypeOf((*NetworkServiceProvider)(nil))
	logger                     = flogging.MustGetLogger("fabric-sdk")
)

// NetworkService models a Fabric Network
type NetworkService struct {
	subscriber events.Subscriber
	fns        driver.FabricNetworkService
	name       string

	channelMutex sync.RWMutex
	channels     map[string]*Channel
}

func NewNetworkService(subscriber events.Subscriber, fns driver.FabricNetworkService, name string) *NetworkService {
	return &NetworkService{subscriber: subscriber, fns: fns, name: name, channels: map[string]*Channel{}}
}

// Channel returns the channel service for the passed id
func (n *NetworkService) Channel(id string) (*Channel, error) {
	n.channelMutex.RLock()
	c, ok := n.channels[id]
	n.channelMutex.RUnlock()
	if ok {
		return c, nil
	}

	n.channelMutex.Lock()
	defer n.channelMutex.Unlock()
	c, ok = n.channels[id]
	if ok {
		return c, nil
	}

	ch, err := n.fns.Channel(id)
	if err != nil {
		return nil, err
	}
	c, ok = n.channels[ch.Name()]
	if ok {
		return c, nil
	}

	c = NewChannel(n.subscriber, n.fns, ch)
	n.channels[ch.Name()] = c

	return c, nil
}

// IdentityProvider returns the identity provider of this network
func (n *NetworkService) IdentityProvider() *IdentityProvider {
	return &IdentityProvider{
		localMembership: n.fns.LocalMembership(),
		ip:              n.fns.IdentityProvider(),
	}
}

// LocalMembership returns the local membership of this network
func (n *NetworkService) LocalMembership() *LocalMembership {
	return &LocalMembership{
		network: n.fns,
	}
}

// Ordering returns the list of known Orderer nodes
func (n *NetworkService) Ordering() *Ordering {
	return &Ordering{network: n.fns}
}

// Name of this network
func (n *NetworkService) Name() string {
	return n.name
}

// ProcessorManager returns the processor manager of this network
func (n *NetworkService) ProcessorManager() *ProcessorManager {
	return &ProcessorManager{pm: n.fns.ProcessorManager()}
}

// TransactionManager returns the transaction manager of this network
func (n *NetworkService) TransactionManager() *TransactionManager {
	return &TransactionManager{fns: n}
}

// SignerService returns the signature service of this network
func (n *NetworkService) SignerService() *SignerService {
	return &SignerService{sigService: n.fns.SignerService()}
}

func (n *NetworkService) ConfigService() *ConfigService {
	return &ConfigService{confService: n.fns.ConfigService()}
}

type NetworkServiceProvider struct {
	fnsProvider     driver.FabricNetworkServiceProvider
	subscriber      events.Subscriber
	mutex           sync.RWMutex
	networkServices map[string]*NetworkService
}

func NewNetworkServiceProvider(fnsProvider driver.FabricNetworkServiceProvider, subscriber events.Subscriber) *NetworkServiceProvider {
	return &NetworkServiceProvider{fnsProvider: fnsProvider, subscriber: subscriber, networkServices: make(map[string]*NetworkService)}
}

func (nsp *NetworkServiceProvider) FabricNetworkService(id string) (*NetworkService, error) {
	nsp.mutex.RLock()
	ns, ok := nsp.networkServices[id]
	nsp.mutex.RUnlock()
	if ok {
		return ns, nil
	}

	nsp.mutex.Lock()
	defer nsp.mutex.Unlock()
	ns, ok = nsp.networkServices[id]
	if ok {
		return ns, nil
	}

	internalFns, err := nsp.fnsProvider.FabricNetworkService(id)
	if err != nil {
		logger.Errorf("Failed to get Fabric Network Service for id [%s]: [%s]", id, err)
		return nil, errors.WithMessagef(err, "Failed to get Fabric Network Service for id [%s]", id)
	}
	ns, ok = nsp.networkServices[internalFns.Name()]
	if ok {
		return ns, nil
	}

	ns = NewNetworkService(nsp.subscriber, internalFns, internalFns.Name())
	nsp.networkServices[id] = ns
	nsp.networkServices[internalFns.Name()] = ns

	return ns, nil
}

func GetNetworkServiceProvider(sp view2.ServiceProvider) (*NetworkServiceProvider, error) {
	s, err := sp.GetService(networkServiceProviderType)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting fabric network service provider")
	}
	return s.(*NetworkServiceProvider), nil
}

func GetFabricNetworkNames(sp view2.ServiceProvider) ([]string, error) {
	provider, err := core.GetFabricNetworkServiceProvider(sp)
	if err != nil {
		return nil, err
	}
	return provider.Names(), nil
}

// GetFabricNetworkService returns the Fabric Network Service for the passed id, nil if not found
func GetFabricNetworkService(sp view2.ServiceProvider, id string) (*NetworkService, error) {
	provider, err := GetNetworkServiceProvider(sp)
	if err != nil {
		return nil, err
	}
	fns, err := provider.FabricNetworkService(id)
	if err != nil {
		return nil, err
	}
	return fns, nil
}

// GetDefaultFNS returns the default Fabric Network Service
func GetDefaultFNS(sp view2.ServiceProvider) (*NetworkService, error) {
	return GetFabricNetworkService(sp, "")
}

// GetDefaultChannel returns the default channel of the default fns
func GetDefaultChannel(sp view2.ServiceProvider) (*NetworkService, *Channel, error) {
	network, err := GetDefaultFNS(sp)
	if err != nil {
		return nil, nil, err
	}
	channel, err := network.Channel("")
	if err != nil {
		return nil, nil, err
	}
	return network, channel, nil
}

// GetChannel returns the requested channel for the passed network
func GetChannel(sp view2.ServiceProvider, network, channel string) (*NetworkService, *Channel, error) {
	fns, err := GetFabricNetworkService(sp, network)
	if err != nil {
		return nil, nil, err
	}
	ch, err := fns.Channel(channel)
	if err != nil {
		return nil, nil, err
	}
	return fns, ch, nil
}
