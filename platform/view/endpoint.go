/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = flogging.MustGetLogger("view-sdk")

// PortName is the type variable for the socket ports
type PortName string

const (
	// ListenPort is the port at which the FSC node might listen for some service
	ListenPort PortName = "Listen"
	// ViewPort is the port on which the View Service Server respond
	ViewPort PortName = "View"
	// P2PPort is the port on which the P2P Communication Layer respond
	P2PPort PortName = "P2P"
)

// PKIResolver extracts public key ids from identities
type PKIResolver interface {
	// GetPKIidOfCert returns the id of the public key contained in the passed identity
	GetPKIidOfCert(peerIdentity view.Identity) []byte
}

// EndpointService provides endpoint-related services
type EndpointService struct {
	es driver.EndpointService
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
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("resolved [%s] to [%s] with ports [%v]", party, id, ports)
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

// IsBoundTo returns true if b was bound to a
func (e *EndpointService) IsBoundTo(a view.Identity, b view.Identity) bool {
	return e.es.IsBoundTo(a, b)
}

// AddResolver adds a resolver for tha passed parameters. The passed id can be retrieved by using the passed name in a call to GetIdentity method.
// The addresses can retrieved by passing the identity in a call to Resolve.
// If a resolver is already bound to the passed name, then the passed identity is linked to the already existing identity. The already existing
// identity is returned
func (e *EndpointService) AddResolver(name string, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error) {
	return e.es.AddResolver(name, domain, addresses, aliases, id)
}

// AddPKIResolver add a new PKI resolver
func (e *EndpointService) AddPKIResolver(pkiResolver PKIResolver) error {
	return e.es.AddPKIResolver(pkiResolver)
}

// GetEndpointService returns an instance of the endpoint service.
// It panics, if no instance is found.
func GetEndpointService(sp ServiceProvider) *EndpointService {
	return &EndpointService{es: driver.GetEndpointService(sp)}
}
