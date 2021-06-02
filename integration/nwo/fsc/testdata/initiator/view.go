/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package initiator

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Factory struct {
}

func (f Factory) NewView(in []byte) (view.View, error) {
	panic("implement me")
}

type Initiator struct {
}

func (i *Initiator) Call(context view.Context) (interface{}, error) {
	panic("implement me")
}
