/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ViewManager manages the lifecycle of views and contexts
type ViewManager interface {
	// NewView returns a new instance of the view identified by the passed id and on input
	NewView(id string, in []byte) (view.View, error)
	// Context returns the context associated to the passed id, an error if not context is found.
	Context(contextID string) (view.Context, error)
	// InitiateView invokes the passed view and returns the result produced by that view
	InitiateView(view view.View) (interface{}, error)
	// InitiateContext initiates a new context for the passed view
	InitiateContext(view view.View) (view.Context, error)
}

// GetViewManager returns an instance of the view manager.
// It panics, if no instance is found.
func GetViewManager(sp ServiceProvider) ViewManager {
	s, err := sp.GetService(reflect.TypeOf((*ViewManager)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(ViewManager)
}
