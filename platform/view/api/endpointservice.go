/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package api

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type PortName string

const (
	ListenPort PortName = "Listen" // Port at which the FSC node might listen for some service
	ViewPort   PortName = "View"   // Port at which the View Service Server respond
	P2PPort    PortName = "P2P"    // Port at which the P2P Communication Layer respond
)

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

	Bind(term view.Identity, ephemeral view.Identity) error
}

func GetEndpointService(ctx ServiceProvider) EndpointService {
	s, err := ctx.GetService(reflect.TypeOf((*EndpointService)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(EndpointService)
}
