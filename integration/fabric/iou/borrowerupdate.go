/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package iou

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Update struct {
	LinearID string
	Amount   uint
}

type UpdateIOUInitiatorView struct {
	Update
}

func (u UpdateIOUInitiatorView) Call(context view.Context) (interface{}, error) {
	// Instantiate a new transaction
	tx, err := state.NewTransaction(context)
	assert.NoError(err)
	tx.SetProposal("iou", "Version-0.0", "update")

	// let's update the IOU on the worldstate
	iouState := &IOUState{}
	assert.NoError(tx.AddInputByLinearID(u.LinearID, iouState))

	// Modify the amount
	iouState.Amount = u.Amount

	err = tx.AddOutput(iouState)
	assert.NoError(err)

	// Collect signature from the lender
	_, err = context.RunView(state.NewCollectEndorsementsView(tx, iouState.Owners()...))
	assert.NoError(err)

	// Send to the ordering service and wait for confirmation
	return context.RunView(state.NewOrderingView(tx))
}
