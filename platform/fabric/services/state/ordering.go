/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/pvt"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

// NewOrderingAndFinalityView returns a view that does the following:
// 1. Sends the passed transaction to the ordering service.
// 2. Waits for the finality of the transaction.
func NewOrderingAndFinalityView(tx *Transaction) view.View {
	return endorser.NewOrderingAndFinalityView(tx.tx)
}

func NewOrderingAndFinalityWithTimeoutView(tx *Transaction, timeout time.Duration) view.View {
	return endorser.NewOrderingAndFinalityWithTimeoutView(tx.tx, timeout)
}

type PrivateOrderingAndFinalityView struct {
	tx *Transaction
}

func (p *PrivateOrderingAndFinalityView) Call(context view.Context) (interface{}, error) {
	txIDBoxed, err := context.RunView(pvt.NewOrderingView(p.tx.tx.Transaction).SetNamespace("pvt"))
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to order transaction [%s]", p.tx.ID())
	}
	return context.RunView(endorser.NewFinalityFor(p.tx.Network(), p.tx.Channel(), txIDBoxed.(string), 0))
}

func NewPrivateOrderingAndFinalityView(tx *Transaction) view.View {
	return &PrivateOrderingAndFinalityView{tx: tx}
}
