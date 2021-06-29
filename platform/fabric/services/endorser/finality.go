/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type finalityView struct {
	tx        *Transaction
	endpoints []view.Identity
}

func (f *finalityView) Call(context view.Context) (interface{}, error) {
	fns := fabric.GetFabricNetworkService(context, f.tx.Network())
	ch, err := fns.Channel(f.tx.Channel())
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting channel [%s:%s]", f.tx.Network(), f.tx.Channel())
	}
	if len(f.endpoints) != 0 {
		return nil, ch.Finality().IsFinalForParties(f.tx.ID(), f.endpoints...)
	}
	return nil, ch.Finality().IsFinal(f.tx.ID())
}

func NewFinalityView(tx *Transaction) *finalityView {
	return &finalityView{tx: tx}
}

func NewFinalityFromView(tx *Transaction, endpoints ...view.Identity) *finalityView {
	return &finalityView{tx: tx, endpoints: endpoints}
}
