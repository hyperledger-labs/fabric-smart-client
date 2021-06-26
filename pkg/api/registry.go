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

	// RegisterResponder binds a responder to an initiator
	RegisterResponder(responder view.View, initiatedBy view.View)

	// RegisterResponderWithIdentity binds the pair <responder, id>
	// with an initiator.
	RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy view.View)
}
