/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

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

//go:generate counterfeiter -o mock/resolver.go -fake-name EndpointService . EndpointService

// EndpointService models the endpoint service
type EndpointService interface {
	// Endpoint returns the known endpoints bound to the passed identity
	Endpoint(party view.Identity) (map[PortName]string, error)

	// Resolve returns the identity the passed identity is bound to.
	// It returns also: the endpoints and the pkiID
	Resolve(party view.Identity) (view.Identity, map[PortName]string, []byte, error)

	// GetIdentity returns an identity bound to either the passed label or public-key identifier.
	GetIdentity(label string, pkiID []byte) (view.Identity, error)

	// Bind binds b to identity a
	Bind(b view.Identity, a view.Identity) error

	// IsBoundTo returns true if b was bound to a
	IsBoundTo(a view.Identity, b view.Identity) bool

	// AddResolver adds a resolver for tha passed parameters. The passed id can be retrieved by using the passed name in a call to GetIdentity method.
	// The addresses can retrieved by passing the identity in a call to Resolve.
	// If a resolver is already bound to the passed name, then the passed identity is linked to the already existing identity. The already existing
	// identity is returned
	AddResolver(name string, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error)

	// AddPKIResolver add a new PKI resolver
	AddPKIResolver(pkiResolver PKIResolver) error
}

// GetEndpointService returns an instance of the endpoint service.
// It panics, if no instance is found.
func GetEndpointService(ctx ServiceProvider) EndpointService {
	s, err := ctx.GetService(reflect.TypeOf((*EndpointService)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(EndpointService)
}
