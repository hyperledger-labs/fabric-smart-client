/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
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

	flp              *finality.Provider
	finalityProvider lazy.Provider[string, *Finality]
}

func NewNetworkService(
	fnsp *fabric.NetworkServiceProvider,
	fabricNetworkService *fabric.NetworkService,
	configProvider config.Provider,
) (*NetworkService, error) {
	qs, err := queryservice.NewRemoteQueryServiceFromConfig(fabricNetworkService.ConfigService())
	if err != nil {
		return nil, errors.Wrap(err, "failed creating remote query service")
	}
	flp := finality.NewListenerManagerProvider(fnsp, configProvider)

	return &NetworkService{
		NetworkService: fabricNetworkService,
		queryService:   NewQueryService(qs),
		flp:            flp,
		finalityProvider: lazy.NewProvider[string, *Finality](func(ch string) (*Finality, error) {
			manager, err := flp.NewManager(fabricNetworkService.Name(), ch)
			if err != nil {
				return nil, err
			}
			return NewFinality(manager), nil
		}),
	}, nil
}

func (ns *NetworkService) FabricNetworkService() *fabric.NetworkService {
	return ns.NetworkService
}

func (ns *NetworkService) QueryService() *QueryService {
	return ns.queryService
}

func (ns *NetworkService) FinalityService() (*Finality, error) {
	return ns.finalityProvider.Get("")
}

type NetworkServiceProvider struct {
	fnsProvider    *fabric.NetworkServiceProvider
	configProvider config.Provider

	providers lazy.Provider[string, *NetworkService]
}

func NewNetworkServiceProvider(fnsProvider *fabric.NetworkServiceProvider, configProvider config.Provider) *NetworkServiceProvider {
	return &NetworkServiceProvider{
		fnsProvider:    fnsProvider,
		configProvider: configProvider,
		providers: lazy.NewProvider[string, *NetworkService](func(id string) (*NetworkService, error) {
			internalFns, err := fnsProvider.FabricNetworkService(id)
			if err != nil {
				logger.Errorf("failed to get Fabric Network Service for id [%s]: [%s]", id, err.Error())
				return nil, errors.WithMessagef(err, "failed to get Fabric Network Service for id [%s]", id)
			}
			ns, err := NewNetworkService(fnsProvider, internalFns, configProvider)
			if err != nil {
				return nil, errors.WithMessagef(err, "failed to create Fabric Network Service for id [%s]", id)
			}
			return ns, nil
		}),
	}
}

func (nsp *NetworkServiceProvider) FabricNetworkServiceProvider() *fabric.NetworkServiceProvider {
	return nsp.fnsProvider
}

func (nsp *NetworkServiceProvider) FabricNetworkService(id string) (*NetworkService, error) {
	return nsp.providers.Get(id)
}

func GetNetworkServiceProvider(sp services.Provider) (*NetworkServiceProvider, error) {
	s, err := sp.GetService(networkServiceProviderType)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting fabric network service provider")
	}
	return s.(*NetworkServiceProvider), nil
}

func GetFabricNetworkNames(sp services.Provider) ([]string, error) {
	return fabric.GetFabricNetworkNames(sp)
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
