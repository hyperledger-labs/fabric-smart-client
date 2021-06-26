/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Registry keeps track of the available view and view factories
type Registry interface {
	// GetIdentifier returns the identifier of the passed view
	GetIdentifier(f view.View) string

	// RegisterFactory binds an id to a View Factory
	RegisterFactory(id string, factory Factory) error

	// RegisterResponder binds a responder to an initiator
	RegisterResponder(responder view.View, initiatedBy view.View)

	// RegisterResponderWithIdentity binds the pair <responder, id>
	// with an initiator.
	RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy view.View)
}

func GetRegistry(sp ServiceProvider) Registry {
	s, err := sp.GetService(reflect.TypeOf((*Registry)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(Registry)
}
