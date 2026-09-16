/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package identity

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type EndpointService interface {
	GetIdentity(label string, pkiID []byte) (view.Identity, error)
	endpoint.Service
}

type Provider interface {
	New(network string) (driver.IdentityProvider, error)
}

type provider struct {
	configProvider  config.Provider
	endpointService EndpointService
}

func NewProvider(configProvider config.Provider, endpointService EndpointService) Provider {
	return &provider{
		configProvider:  configProvider,
		endpointService: endpointService,
	}
}

func (p *provider) New(network string) (driver.IdentityProvider, error) {
	// Endpoint service
	c, err := p.configProvider.GetConfig(network)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get config")
	}
	resolverService, err := endpoint.NewResolverService(c, p.endpointService)
	if err != nil {
		return nil, errors.Wrap(err, "failed instantiating fabric endpoint resolver")
	}
	if err := resolverService.LoadResolvers(); err != nil {
		return nil, errors.Wrap(err, "failed loading fabric endpoint resolvers")
	}
	endpointService, err := endpoint.NewResolver(resolverService, p.endpointService)
	if err != nil {
		return nil, errors.Wrap(err, "failed loading endpoint service")
	}

	// Identity Manager
	idProvider, err := id.NewProvider(endpointService)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating identity provider")
	}
	return idProvider, nil
}
