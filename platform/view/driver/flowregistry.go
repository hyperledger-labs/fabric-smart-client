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

	// RegisterResponder binds a responder to an initiator.
	// The responder is the view that will be called when the initiator (initiatedBy) contacts the FSC node where
	// this RegisterResponder is invoked.
	// The argument initiatedBy can be a view or a view identifier.
	// If a view is passed, its identifier is computed and used to register the responder.
	RegisterResponder(responder view.View, initiatedBy interface{}) error

	// RegisterResponderWithIdentity binds the pair <responder, id> to an initiator.
	// The responder is the view that will be called when the initiator (initiatedBy) contacts the FSC node where
	// this RegisterResponderWithIdentity is invoked.
	// The argument initiatedBy can be a view or a view identifier.
	// If a view is passed, its identifier is computed and used to register the responder.
	RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy interface{}) error

	// GetResponder returns the responder for the passed initiator.
	GetResponder(initiatedBy interface{}) (view.View, error)
}

func GetRegistry(sp ServiceProvider) Registry {
	s, err := sp.GetService(reflect.TypeOf((*Registry)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(Registry)
}
