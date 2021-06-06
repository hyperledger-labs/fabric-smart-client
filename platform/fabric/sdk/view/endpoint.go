/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package view

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endpoint"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	endpoint2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type EndpointService interface {
	Bind(longTerm view.Identity, ephemeral view.Identity) error
	AddResolver(name string, domain string, addresses map[string]string, aliases []string, id []byte) error
	AddPKIResolver(resolver endpoint2.PKIResolver) error
}

type endpointService struct {
	es EndpointService
}

func NewEndpointService(sp view2.ServiceProvider) (*endpointService, error) {
	s, err := sp.GetService(reflect.TypeOf((*EndpointService)(nil)))
	if err != nil {
		return nil, err
	}
	return &endpointService{es: s.(EndpointService)}, nil
}

func (e *endpointService) Bind(longTerm view.Identity, ephemeral view.Identity) error {
	return e.es.Bind(longTerm, ephemeral)
}

func (e *endpointService) AddResolver(name string, domain string, addresses map[string]string, aliases []string, id []byte) error {
	return e.es.AddResolver(name, domain, addresses, aliases, id)
}

func (e *endpointService) AddPKIResolver(resolver endpoint.PKIResolver) error {
	return e.es.AddPKIResolver(resolver)
}
