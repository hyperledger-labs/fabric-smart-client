/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// View wraps a callable function.
type View interface {
	// Call invokes the View on input the passed argument.
	// It returns a result and error in case of failure.
	Call(context view.Context) (interface{}, error)
}

// Factory is used to create instances of the View interface
type Factory interface {
	// NewView returns an instance of the View interface build using the passed argument.
	NewView(in []byte) (view.View, error)
}

// Registry keeps track of the available view and view factories
type Registry struct {
	registry driver.Registry
}

// GetIdentifier returns the identifier of the passed view
func (r *Registry) GetIdentifier(f View) string {
	return r.registry.GetIdentifier(f)
}

// RegisterFactory binds an id to a View Factory
func (r *Registry) RegisterFactory(id string, factory Factory) error {
	return r.registry.RegisterFactory(id, factory)
}

// RegisterResponder binds a responder to an initiator.
// The responder is the view that will be called when the initiator (initiatedBy) contacts the FSC node where
// this RegisterResponder is invoked.
// The argument initiatedBy can be a view or a view identifier.
// If a view is passed, its identifier is computed and used to register the responder.
func (r *Registry) RegisterResponder(responder View, initiatedBy interface{}) error {
	return r.registry.RegisterResponder(responder, initiatedBy)
}

// GetResponder returns the responder for the passed initiator.
func (r *Registry) GetResponder(initiatedBy interface{}) (View, error) {
	return r.registry.GetResponder(initiatedBy)
}

// RegisterResponderWithIdentity binds the pair <responder, id> to an initiator.
// The responder is the view that will be called when the initiator (initiatedBy) contacts the FSC node where
// this RegisterResponderWithIdentity is invoked.
// The argument initiatedBy can be a view or a view identifier.
// If a view is passed, its identifier is computed and used to register the responder.
func (r *Registry) RegisterResponderWithIdentity(responder View, id view.Identity, initiatedBy interface{}) error {
	return r.registry.RegisterResponderWithIdentity(responder, id, initiatedBy)
}

// GetRegistry returns an instance of the view registry.
// It panics, if no instance is found.
func GetRegistry(sp ServiceProvider) *Registry {
	s, err := sp.GetService(reflect.TypeOf((*driver.Registry)(nil)))
	if err != nil {
		panic(err)
	}
	return &Registry{registry: s.(driver.Registry)}
}
