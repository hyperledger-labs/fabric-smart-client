/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ViewManager models the view manager for the view service server.
type ViewManager interface {
	// NewView returns a new view instance for the given ID and input.
	NewView(id string, in []byte) (view.View, error)
	// InitiateView initiates a protocol for the given view.
	InitiateView(ctx context.Context, view view.View) (any, error)
	// InitiateContext initiates a view context for the given view.
	InitiateContext(ctx context.Context, view view.View) (view.Context, error)
	// DeleteContext removes a context from the manager and calls Dispose on the context.
	DeleteContext(contextID string)
}
