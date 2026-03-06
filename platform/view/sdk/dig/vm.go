/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// StartableViewManager extends the ViewManager interface with a method to set the root context.
type StartableViewManager interface {
	// NewView returns a new view instance for the given ID and input.
	NewView(id string, in []byte) (view.View, error)
	// InitiateView initiates a protocol for the given view.
	InitiateView(view view.View, ctx context.Context) (interface{}, error)
	// InitiateContext initiates a view context for the given view.
	InitiateContext(view view.View) (view.Context, error)
	// DeleteContext removes a context from the manager and calls Dispose on the context.
	DeleteContext(contextID string)
	// SetContext sets the root context.
	SetContext(ctx context.Context)
}
