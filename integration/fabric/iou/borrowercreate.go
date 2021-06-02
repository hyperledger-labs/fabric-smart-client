/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package iou

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Create struct {
	Amount uint
}

type CreateIOUInitiatorView struct {
	Create
}

func (i *CreateIOUInitiatorView) Call(context view.Context) (interface{}, error) {
	// Instantiate a new transaction
	tx, err := state.NewTransaction(context)
	assert.NoError(err)
	tx.SetProposal("iou", "Version-0.0", "create")

	// Instantiate the state
	lender := fabric.GetIdentityProvider(context).Identity("lender")
	iou := &IOUState{
		Amount:  i.Amount,
		Parties: []view.Identity{context.Me(), lender},
	}

	// Add the state to the transaction
	assert.NoError(tx.AddOutput(iou))

	// Collect endorsements
	_, err = context.RunView(state.NewCollectEndorsementsView(tx, iou.Parties...))
	assert.NoError(err)

	// Send transaction and wait for ordering
	_, err = context.RunView(state.NewOrderingView(tx))
	assert.NoError(err)

	// Return the state ID
	return iou.LinearID, nil
}
