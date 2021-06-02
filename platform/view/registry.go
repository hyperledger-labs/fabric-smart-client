/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package view

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type View interface {
	Call(context view.Context) (interface{}, error)
}

type Factory interface {
	// NewView returns an instance of the View interface build using the passed argument.
	NewView(in []byte) (view.View, error)
}

type Registry struct {
	registry api.Registry
}

func (r *Registry) GetIdentifier(f View) string {
	return r.registry.GetIdentifier(f)
}

func (r *Registry) RegisterFactory(id string, factory Factory) error {
	return r.registry.RegisterFactory(id, factory)
}

func (r *Registry) RegisterResponder(responder View, initiatedBy View) {
	r.registry.RegisterResponder(responder, initiatedBy)
}

func (r *Registry) RegisterResponderWithIdentity(responder View, id view.Identity, initiatedBy View) {
	r.registry.RegisterResponderWithIdentity(responder, id, initiatedBy)
}

func GetRegistry(sp ServiceProvider) *Registry {
	s, err := sp.GetService(reflect.TypeOf((*api.Registry)(nil)))
	if err != nil {
		panic(err)
	}
	return &Registry{registry: s.(api.Registry)}
}
