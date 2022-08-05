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

type orderingView struct {
	tx       *Transaction
	finality bool
}

func (o *orderingView) Call(context view.Context) (interface{}, error) {
	fns := fabric.GetFabricNetworkService(context, o.tx.Network())
	if fns == nil {
		return nil, errors.Errorf("fabric network service [%s] not found", o.tx.Network())
	}
	tx := o.tx
	if err := fns.Ordering().Broadcast(tx.Transaction); err != nil {
		return nil, errors.WithMessagef(err, "failed broadcasting to [%s:%s]", o.tx.Network(), o.tx.Channel())
	}
	if o.finality {
		ch, err := fns.Channel(o.tx.Channel())
		if err != nil {
			return nil, errors.WithMessagef(err, "failed getting channel [%s:%s]", o.tx.Network(), o.tx.Channel())
		}
		if err := ch.Finality().IsFinal(tx.ID()); err != nil {
			return nil, errors.WithMessagef(err, "failed asking finality of [%s] to [%s:%s]", tx.ID(), o.tx.Network(), o.tx.Channel())
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
