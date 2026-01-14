/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx

import (
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

var (
	networkServiceProviderType = reflect.TypeOf((*NetworkServiceProvider)(nil))
	logger                     = logging.MustGetLogger()
)

// NetworkService models a Fabric Network
type NetworkService struct {
	*fabric.NetworkService
	queryService *QueryService
}

func NewNetworkService(fabricNetworkService *fabric.NetworkService) (*NetworkService, error) {
	qs, err := queryservice.NewRemoteQueryServiceFromConfig(fabricNetworkService.ConfigService())
	if err != nil {
		return nil, errors.Wrap(err, "failed creating remote query service")
	}
	return &NetworkService{
		NetworkService: fabricNetworkService,
		queryService:   NewQueryService(qs),
	}, nil
}

type NetworkServiceProvider struct {
	fnsProvider     *fabric.NetworkServiceProvider
	mutex           sync.RWMutex
	networkServices map[string]*NetworkService
}

func NewNetworkServiceProvider(fnsProvider *fabric.NetworkServiceProvider) *NetworkServiceProvider {
	return &NetworkServiceProvider{fnsProvider: fnsProvider, networkServices: make(map[string]*NetworkService)}
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
		logger.Errorf("failed to get Fabric Network Service for id [%s]: [%s]", id, err.Error())
		return nil, errors.WithMessagef(err, "failed to get Fabric Network Service for id [%s]", id)
	}
	ns, ok = nsp.networkServices[internalFns.Name()]
	if ok {
		return ns, nil
	}

	ns, err = NewNetworkService(internalFns)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create Fabric Network Service for id [%s]", id)
	}
	nsp.networkServices[id] = ns
	nsp.networkServices[internalFns.Name()] = ns

	return ns, nil
}

func GetNetworkServiceProvider(sp services.Provider) (*NetworkServiceProvider, error) {
	s, err := sp.GetService(networkServiceProviderType)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting fabric network service provider")
	}
	return s.(*NetworkServiceProvider), nil
}

func GetFabricNetworkNames(sp services.Provider) ([]string, error) {
	provider, err := core.GetFabricNetworkServiceProvider(sp)
	if err != nil {
		return nil, err
	}
	return provider.Names(), nil
}

// GetFabricNetworkService returns the Fabric Network Service for the passed id, nil if not found
func GetFabricNetworkService(sp services.Provider, id string) (*NetworkService, error) {
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
func GetDefaultFNS(sp services.Provider) (*NetworkService, error) {
	return GetFabricNetworkService(sp, "")
}

// GetDefaultChannel returns the default channel of the default fns
func GetDefaultChannel(sp services.Provider) (*NetworkService, *fabric.Channel, error) {
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
func GetChannel(sp services.Provider, network, channel string) (*NetworkService, *fabric.Channel, error) {
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
