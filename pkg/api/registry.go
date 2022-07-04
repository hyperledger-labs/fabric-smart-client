/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

// Factory is used to create instances of the View interface
type Factory interface {
	// NewView returns an instance of the View interface build using the passed argument.
	NewView(in []byte) (view.View, error)
}

// ViewRegistry keeps track of the available views and view factories
type ViewRegistry interface {
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
	RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy view.View) error
}
