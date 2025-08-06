/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type orderingView struct {
	tx       *Transaction
	finality bool
	timeout  time.Duration
}

func (o *orderingView) Call(ctx view.Context) (interface{}, error) {
	fns, err := fabric.GetFabricNetworkService(ctx, o.tx.Network())
	if err != nil {
		return nil, errors.WithMessagef(err, "fabric network service [%s] not found", o.tx.Network())
	}
	tx := o.tx
	if err := fns.Ordering().Broadcast(ctx.Context(), tx.Transaction); err != nil {
		return nil, errors.WithMessagef(err, "failed broadcasting to [%s:%s]", o.tx.Network(), o.tx.Channel())
	}
	if o.finality {
		return ctx.RunView(NewFinalityWithTimeoutView(tx, o.timeout))
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
