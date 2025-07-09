/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ViewManager interface {
	NewView(id string, in []byte) (view.View, error)
	InitiateView(view view.View, ctx context.Context) (interface{}, error)
	InitiateContext(view view.View) (view.Context, error)
}
