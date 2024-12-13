/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package identity

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	vdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
)

type NamedIdentityLoader struct {
	Name string
	driver.IdentityLoader
}

type MSPManagerProvider interface {
	New(network string) (fdriver.LocalMembership, error)
}

type localMSPManagerProvider struct {
	configProvider      config.Provider
	endpointService     driver.BinderService
	sigService          driver.SignerService
	identityLoaders     []NamedIdentityLoader
	deserializerManager driver.DeserializerManager
	idProvider          vdriver.IdentityProvider
	kvss                *kvs.KVS
}

func NewMSPManagerProvider(
	configProvider config.Provider,
	endpointService EndpointService,
	sigService driver.SignerService,
	identityLoaders []NamedIdentityLoader,
	deserializerManager driver.DeserializerManager,
	idProvider vdriver.IdentityProvider,
	kvss *kvs.KVS,
) *localMSPManagerProvider {
	return &localMSPManagerProvider{
		configProvider:      configProvider,
		endpointService:     endpointService,
		sigService:          sigService,
		identityLoaders:     identityLoaders,
		deserializerManager: deserializerManager,
		idProvider:          idProvider,
		kvss:                kvss,
	}
}

func (p *localMSPManagerProvider) New(network string) (fdriver.LocalMembership, error) {
	genericConfig, err := p.configProvider.GetConfig(network)
	if err != nil {
		return nil, err
	}

	// Local MSP Manager
	mspService := msp.NewLocalMSPManager(
		genericConfig,
		p.kvss,
		p.sigService,
		p.endpointService,
		p.idProvider.DefaultIdentity(),
		p.deserializerManager,
		genericConfig.MSPCacheSize(),
	)
	for _, loader := range p.identityLoaders {
		mspService.PutIdentityLoader(loader.Name, loader.IdentityLoader)
	}
	if err := mspService.Load(); err != nil {
		return nil, fmt.Errorf("failed loading local msp service: %w", err)
	}
	return mspService, nil
}
