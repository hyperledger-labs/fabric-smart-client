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

type View interface {
	Call(context view.Context) (interface{}, error)
}

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

// RegisterResponder binds a responder to an initiator
func (r *Registry) RegisterResponder(responder View, initiatedBy View) {
	r.registry.RegisterResponder(responder, initiatedBy)
}

// RegisterResponderWithIdentity binds the pair <responder, id>
// with an initiator.
func (r *Registry) RegisterResponderWithIdentity(responder View, id view.Identity, initiatedBy View) {
	r.registry.RegisterResponderWithIdentity(responder, id, initiatedBy)
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
