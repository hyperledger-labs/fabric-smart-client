/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package view

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type PortName string

const (
	ListenPort PortName = "Listen" // Port at which the FSC node might listen for some service
	ViewPort   PortName = "View"   // Port at which the View Service Server respond
	P2PPort    PortName = "P2P"    // Port at which the P2P Communication Layer respond
)

// EndpointService provides endpoint-related services
type EndpointService struct {
	es api.EndpointService
}

// Endpoint returns the endpoint of the passed identity
func (e *EndpointService) Endpoint(party view.Identity) (map[PortName]string, error) {
	res, err := e.es.Endpoint(party)
	if err != nil {
		return nil, err
	}
	out := map[PortName]string{}
	for name, s := range res {
		out[PortName(name)] = s
	}
	return out, nil
}

// Resolve returns the endpoints of the passed identity.
// If the passed identity does not have any endpoint set, the service checks
// if the passed identity is bound to another identity that is returned together with its endpoints and public-key identifier.
func (e *EndpointService) Resolve(party view.Identity) (view.Identity, map[PortName]string, []byte, error) {
	id, ports, raw, err := e.es.Resolve(party)
	if err != nil {
		return nil, nil, nil, err
	}
	out := map[PortName]string{}
	for name, s := range ports {
		out[PortName(name)] = s
	}
	return id, out, raw, nil
}

func (e *EndpointService) ResolveIdentities(endpoints ...string) ([]view.Identity, error) {
	var ids []view.Identity
	for _, endpoint := range endpoints {
		id, err := e.es.GetIdentity(endpoint, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot find the idnetity at %s", endpoint)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// GetIdentity returns an identity bound to either the passed label or public-key identifier.
func (e *EndpointService) GetIdentity(label string, pkiID []byte) (view.Identity, error) {
	return e.es.GetIdentity(label, pkiID)
}

// Bind associated a 'long term' identity to an 'ephemeral' one.
// In more general terms, Bind binds any identity to another.
func (e *EndpointService) Bind(longTerm view.Identity, ephemeral view.Identity) error {
	return e.es.Bind(longTerm, ephemeral)
}

func GetEndpointService(sp ServiceProvider) *EndpointService {
	return &EndpointService{es: api.GetEndpointService(sp)}
}
