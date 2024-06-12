/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package identity

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type EndpointService interface {
	generic.EndpointService
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
		return nil, fmt.Errorf("failed to get config: %w", err)
	}
	resolverService, err := endpoint.NewResolverService(c, p.endpointService)
	if err != nil {
		return nil, fmt.Errorf("failed instantiating fabric endpoint resolver: %w", err)
	}
	if err := resolverService.LoadResolvers(); err != nil {
		return nil, fmt.Errorf("failed loading fabric endpoint resolvers: %w", err)
	}
	endpointService, err := generic.NewEndpointResolver(resolverService, p.endpointService)
	if err != nil {
		return nil, fmt.Errorf("failed loading endpoint service: %w", err)
	}

	// Identity Manager
	idProvider, err := id.NewProvider(endpointService)
	if err != nil {
		return nil, fmt.Errorf("failed creating identity provider: %w", err)
	}
	return idProvider, nil
}
