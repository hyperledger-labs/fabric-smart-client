/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type orderingView struct {
	tx                  *Transaction
	waitForEventTimeout time.Duration
	finality            bool
}

func (o *orderingView) Call(context view.Context) (interface{}, error) {
	tx := o.tx
	if err := fabric.GetDefaultNetwork(context).Ordering().Broadcast(tx.Transaction); err != nil {
		return nil, err
	}
	if o.finality {
		if err := fabric.GetChannelDefaultNetwork(context, tx.Channel()).Finality().IsFinal(tx.ID()); err != nil {
			return nil, err
		}
	}
	return tx, nil
}

func NewOrderingAndFinalityView(tx *Transaction) *orderingView {
	return &orderingView{tx: tx, finality: true}
}

func NewOrderingView(tx *Transaction) *orderingView {
	return &orderingView{tx: tx, finality: false}
}
