/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package iou

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type CreateIOUResponderView struct{}

func (i *CreateIOUResponderView) Call(context view.Context) (interface{}, error) {
	// Unmarshall the received transaction
	tx, err := state.ReceiveTransaction(context)
	assert.NoError(err, "failed receiving transaction")

	// Inspect Proposal
	function, args := tx.FunctionAndParameters()
	assert.Equal(0, len(args), "invalid number of arguments, expected 0, was %d", len(args))

	switch function {
	case "create":
		assert.Equal(0, tx.NumInputs(), "invalid number of inputs, "+
			"expected 0, was %d", tx.NumInputs())
		assert.Equal(1, tx.NumOutputs(), "invalid number of outputs, "+
			"expected 1, was %d", tx.NumInputs())

		iouState := &IOUState{}
		assert.NoError(tx.GetOutputAt(0, iouState))
		if iouState.Amount < 5 {
			return nil, errors.Errorf("invalid amount, "+
				"expected at least 5, was %d", iouState.Amount)
		}
		assert.Equal(2, iouState.Owners().Count(), "invalid state, "+
			"expected 2 identities, was %d", iouState.Owners().Count())
		assert.True(iouState.Owners().Contain(context.Me()), "invalid state,"+
			"it does not contain lender identity")
		assert.False(iouState.Owners().Others(context.Me()).Contain(context.Me()), "invalid state,"+
			"it does not contain borrower identity")
		assert.NoError(tx.HasBeenEndorsedBy(iouState.Owners().Others(context.Me())...),
			"the borrower has not endorsed")
	default:
		return nil, errors.Errorf("invalid command, "+
			"expected [create], was [%s]", function)
	}

	// Send it back to the initiator signed
	_, err = context.RunView(state.NewEndorseView(tx))
	assert.NoError(err)

	// Wait for finality
	return context.RunView(state.NewFinalityView(tx))
}
