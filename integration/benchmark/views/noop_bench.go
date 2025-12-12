/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type NoopView struct {
}

func (q *NoopView) Call(viewCtx view.Context) (interface{}, error) {
	return "OK", nil
}

type NoopViewFactory struct{}

func (c *NoopViewFactory) NewView(_ []byte) (view.View, error) {
	return &NoopView{}, nil
}
