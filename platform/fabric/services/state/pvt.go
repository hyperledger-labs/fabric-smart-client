/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/pvt"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type PrivateOrderingAndFinalityView struct {
	tx *Transaction
}

func NewPrivateOrderingAndFinalityView(tx *Transaction) view.View {
	return &PrivateOrderingAndFinalityView{tx: tx}
}

func (p *PrivateOrderingAndFinalityView) Call(context view.Context) (interface{}, error) {
	txIDBoxed, err := context.RunView(pvt.NewOrderingView(p.tx.tx.Transaction).SetNamespace("pvt"))
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to order transaction [%s]", p.tx.ID())
	}
	return context.RunView(endorser.NewFinalityFor(p.tx.Network(), p.tx.Channel(), txIDBoxed.(string), 0))
}

type PrivateCollectEndorsementsView struct {
	tx      *Transaction
	parties []view.Identity
}

// NewPrivateCollectEndorsementsView returns a view that does the following:
// 1. It contacts each passed party sequentially and sends the marshalled version of the passed transaction.
// 2. It waits for a response containing either the endorsement of the transaction or an error.
func NewPrivateCollectEndorsementsView(tx *Transaction, parties ...view.Identity) view.View {
	return &PrivateCollectEndorsementsView{
		tx:      tx,
		parties: parties,
	}
}

func (p *PrivateCollectEndorsementsView) Call(context view.Context) (interface{}, error) {
	_, err := context.RunView(endorser.NewCollectEndorsementsView(p.tx.tx, p.parties...))
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func NewPrivateCollectApprovesView(tx *Transaction, parties ...view.Identity) view.View {
	return endorser.NewCollectEndorsementsView(tx.tx, parties...)
}

// NewPrivateEndorseView returns a view that does the following:
// 1. It signs the transaction with the signing key of each passed identity
// 2. Send the transaction back on the context's session.
func NewPrivateEndorseView(tx *Transaction, ids ...view.Identity) view.View {
	return endorser.NewEndorseView(tx.tx, ids...)
}
