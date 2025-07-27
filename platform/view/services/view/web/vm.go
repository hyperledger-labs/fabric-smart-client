/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ViewManager interface {
	Context() context.Context
	NewView(id string, in []byte) (view.View, error)
	InitiateView(ctx context.Context, view view.View) (interface{}, error)
	InitiateContext(ctx context.Context, view view.View) (view.Context, error)
}
