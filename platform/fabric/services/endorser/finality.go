/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package endorser

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type finalityView struct {
	tx        *Transaction
	endpoints []view.Identity
}

func (f *finalityView) Call(context view.Context) (interface{}, error) {
	ch := fabric.GetChannelDefaultNetwork(context, f.tx.Channel())
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
