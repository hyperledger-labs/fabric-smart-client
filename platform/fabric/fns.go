/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/pkg/errors"
)

var (
	networkServiceProviderType = reflect.TypeOf((*NetworkServiceProvider)(nil))
	logger                     = flogging.MustGetLogger("fabric-sdk")
)

// NetworkService models a Fabric Network
type NetworkService struct {
	SP   view2.ServiceProvider
	fns  driver.FabricNetworkService
	name string

	channelMutex sync.RWMutex
	channels     map[string]*Channel
}

func NewNetworkService(SP view2.ServiceProvider, fns driver.FabricNetworkService, name string) *NetworkService {
	return &NetworkService{SP: SP, fns: fns, name: name, channels: map[string]*Channel{}}
}

// DefaultChannel returns the name of the default channel
func (n *NetworkService) DefaultChannel() string {
	return n.fns.DefaultChannel()
}

// Channels returns the channel names
func (n *NetworkService) Channels() []string {
	return n.fns.Channels()
}

// Peers returns the list of known Peer nodes
func (n *NetworkService) Peers() []*grpc.ConnectionConfig {
	return n.fns.Peers()
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

	c = NewChannel(n.SP, n.fns, ch)
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
	sp              view2.ServiceProvider
	mutex           sync.RWMutex
	networkServices map[string]*NetworkService
}

func NewNetworkServiceProvider(sp view2.ServiceProvider) *NetworkServiceProvider {
	return &NetworkServiceProvider{sp: sp, networkServices: make(map[string]*NetworkService)}
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

	provider := core.GetFabricNetworkServiceProvider(nsp.sp)
	if provider == nil {
		return nil, errors.New("no Fabric Network Service Provider found")
	}
	internalFns, err := provider.FabricNetworkService(id)
	if err != nil {
		logger.Errorf("Failed to get Fabric Network Service for id [%s]: [%s]", id, err)
		return nil, errors.WithMessagef(err, "Failed to get Fabric Network Service for id [%s]", id)
	}
	ns, ok = nsp.networkServices[internalFns.Name()]
	if ok {
		return ns, nil
	}

	ns = NewNetworkService(nsp.sp, internalFns, internalFns.Name())
	nsp.networkServices[id] = ns
	nsp.networkServices[internalFns.Name()] = ns

	return ns, nil
}

func GetNetworkServiceProvider(sp view2.ServiceProvider) *NetworkServiceProvider {
	s, err := sp.GetService(networkServiceProviderType)
	if err != nil {
		logger.Warnf("failed getting fabric network service provider: %s", err)
		return nil
	}
	return s.(*NetworkServiceProvider)
}

func GetFabricNetworkNames(sp view2.ServiceProvider) []string {
	provider := core.GetFabricNetworkServiceProvider(sp)
	if provider == nil {
		return nil
	}
	return provider.Names()
}

// GetFabricNetworkService returns the Fabric Network Service for the passed id, nil if not found
func GetFabricNetworkService(sp view2.ServiceProvider, id string) *NetworkService {
	provider := GetNetworkServiceProvider(sp)
	if provider == nil {
		return nil
	}
	fns, err := provider.FabricNetworkService(id)
	if err != nil {
		logger.Warnf("Failed to get Fabric Network Service for id [%s]: [%s]", id, err)
		return nil
	}
	return fns
}

// GetDefaultFNS returns the default Fabric Network Service
func GetDefaultFNS(sp view2.ServiceProvider) *NetworkService {
	return GetFabricNetworkService(sp, "")
}

// GetDefaultChannel returns the default channel of the default fns
func GetDefaultChannel(sp view2.ServiceProvider) *Channel {
	network := GetDefaultFNS(sp)
	channel, err := network.Channel("")
	if err != nil {
		panic(err)
	}

	return channel
}

// GetDefaultIdentityProvider returns the identity provider of the default fabric network service
func GetDefaultIdentityProvider(sp view2.ServiceProvider) *IdentityProvider {
	return GetDefaultFNS(sp).IdentityProvider()
}

// GetDefaultLocalMembership returns the local membership of the default fabric network service
func GetDefaultLocalMembership(sp view2.ServiceProvider) *LocalMembership {
	return GetDefaultFNS(sp).LocalMembership()
}

// GetChannel returns the requested channel for the passed network
func GetChannel(sp view2.ServiceProvider, network, channel string) *Channel {
	fns := GetFabricNetworkService(sp, network)
	if fns == nil {
		panic(fmt.Sprintf("fabric network service [%s] not found", network))
	}
	ch, err := fns.Channel(channel)
	if err != nil {
		panic(err)
	}

	return ch
}

// GetVault returns the vualt for the requested channel for the passed network
func GetVault(sp view2.ServiceProvider, network, channel string) *Vault {
	fns := GetFabricNetworkService(sp, network)
	if fns == nil {
		panic(fmt.Sprintf("fabric network service [%s] not found", network))
	}
	ch, err := fns.Channel(channel)
	if err != nil {
		panic(err)
	}

	return ch.Vault()
}

// GetIdentityProvider returns the identity provider for the passed network
func GetIdentityProvider(sp view2.ServiceProvider, network string) *IdentityProvider {
	return GetFabricNetworkService(sp, network).IdentityProvider()
}

// GetLocalMembership returns the local membership for the passed network
func GetLocalMembership(sp view2.ServiceProvider, network string) *LocalMembership {
	return GetFabricNetworkService(sp, network).LocalMembership()
}
