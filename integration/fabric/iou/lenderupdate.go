/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package iou

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type UpdateIOUResponderView struct{}

func (i *UpdateIOUResponderView) Call(context view.Context) (interface{}, error) {
	// Unmarshall the received transaction
	tx, err := state.ReceiveTransaction(context)
	assert.NoError(err, "failed receiving transaction")

	// Inspect Proposal
	function, args := tx.FunctionAndParameters()
	assert.Equal(0, len(args), "invalid number of arguments, expected 0, was %d", len(args))

	switch function {
	case "update":
		assert.Equal(1, tx.NumInputs(), "invalid number of inputs, "+
			"expected 1, was %d", tx.NumInputs())
		assert.Equal(1, tx.NumOutputs(), "invalid number of outputs, "+
			"expected 1, was %d", tx.NumInputs())
		inState := &IOUState{}
		assert.NoError(tx.GetInputAt(0, inState))
		outState := &IOUState{}
		assert.NoError(tx.GetOutputAt(0, outState))

		assert.Equal(inState.LinearID, outState.LinearID, "invalid state id, "+
			"[%s] != [%s]", inState.LinearID, outState.LinearID)
		if outState.Amount >= inState.Amount {
			return nil, errors.Errorf("invalid amount, "+
				"[%d] expected to be less or equal [%d]", outState.Amount, inState.Amount)
		}
		assert.True(inState.Owners().Match(outState.Owners()), "invalid owners, "+
			"input and output should have the same owners")
		assert.NoError(tx.HasBeenEndorsedBy(inState.Owners().Others(
			fabric.GetIdentityProvider(context).DefaultIdentity(),
		)...),
			"the borrower has not endorsed")
	default:
		return nil, errors.Errorf("invalid command, "+
			"expected [create], was [%s]", function)
	}

	// Send it back to the sender signed
	_, err = context.RunView(state.NewEndorseView(tx))
	assert.NoError(err)

	// Wait for confirmation from the ordering service
	return context.RunView(state.NewFinalityView(tx))
}
