/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type StartableViewManager interface {
	NewView(id string, in []byte) (view.View, error)
	InitiateView(view view.View, ctx context.Context) (interface{}, error)
	InitiateContext(view view.View) (view.Context, error)
	InitiateContextWithIdentity(view view.View, id view.Identity) (view.Context, error)
	InitiateContextFrom(ctx context.Context, view view.View, id view.Identity, contextID string) (view.Context, error)
	Context(contextID string) (view.Context, error)
	Start(ctx context.Context)
}
