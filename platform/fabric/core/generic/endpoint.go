/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type EndpointService interface {
	GetIdentity(label string, pkiID []byte) (view.Identity, error)
}

type Resolver interface {
	GetIdentity(label string) view.Identity
}

type endpointResolver struct {
	resolver Resolver
	es       EndpointService
}

func NewEndpointResolver(resolver Resolver, es EndpointService) (*endpointResolver, error) {
	return &endpointResolver{
		resolver: resolver,
		es:       es,
	}, nil
}

func (e *endpointResolver) GetIdentity(label string) (view.Identity, error) {
	res := e.resolver.GetIdentity(label)
	if res.IsNone() {
		// lookup for a level up
		return e.es.GetIdentity(label, nil)

	}
	return res, nil
}
