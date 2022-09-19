/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type orderingView struct {
	tx       *Transaction
	finality bool
	timeout  time.Duration
}

func (o *orderingView) Call(ctx view.Context) (interface{}, error) {
	fns := fabric.GetFabricNetworkService(ctx, o.tx.Network())
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
		c := ctx.Context()
		if o.timeout != 0 {
			var cancel context.CancelFunc
			c, cancel = context.WithTimeout(c, o.timeout)
			defer cancel()
		}
		if err := ch.Finality().IsFinal(c, tx.ID()); err != nil {
			return nil, errors.WithMessagef(err, "failed asking finality of [%s] to [%s:%s]", tx.ID(), o.tx.Network(), o.tx.Channel())
		}
	}
	return tx, nil
}

func NewOrderingAndFinalityView(tx *Transaction) *orderingView {
	return &orderingView{tx: tx, finality: true}
}

func NewOrderingAndFinalityWithTimeoutView(tx *Transaction, timeout time.Duration) *orderingView {
	return &orderingView{tx: tx, finality: true, timeout: timeout}
}

func NewOrderingView(tx *Transaction) *orderingView {
	return &orderingView{tx: tx, finality: false}
}
