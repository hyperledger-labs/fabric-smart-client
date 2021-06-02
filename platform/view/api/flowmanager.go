/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package api

import (
	"context"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ViewManager interface {
	NewView(id string, in []byte) (view.View, error)
	Context(contextID string) (view.Context, error)
	InitiateView(view view.View) (interface{}, error)
	InitiateContext(view view.View) (view.Context, error)
	Start(ctx context.Context)
}

func GetViewManager(sp ServiceProvider) ViewManager {
	s, err := sp.GetService(reflect.TypeOf((*ViewManager)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(ViewManager)
}
