/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package views

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type CreateIOUResponderView struct{}

func (i *CreateIOUResponderView) Call(context view.Context) (interface{}, error) {
	// As a first step, the lender responds to the request to exchange recipient identities.
	lender, borrower, err := state.RespondExchangeRecipientIdentities(context)
	assert.NoError(err, "failed exchanging recipient identities")

	// When the borrower runs the CollectEndorsementsView, at some point, the borrower sends the assembled transaction
	// to the lender. Therefore, the lender waits to receive the transaction.
	tx, err := state.ReceiveTransaction(context)
	assert.NoError(err, "failed receiving transaction")

	// The lender can now inspect the transaction to ensure it is as expected.
	// Here are examples of possible checks

	// Namespaces are properly populated
	assert.Equal(1, len(tx.Namespaces()), "expected only one namespace")
	assert.Equal("iou", tx.Namespaces()[0], "expected the [iou] namespace, got [%s]", tx.Namespaces()[0])

	// Commands are properly populated
	assert.Equal(1, tx.Commands().Count(), "expected only a single command, got [%s]", tx.Commands().Count())
	switch command := tx.Commands().At(0); command.Name {
	case "create":
		// If the create command is attached to the transaction then...

		// No inputs expected. The single output at index 0 should be an IOU state
		assert.Equal(0, tx.NumInputs(), "invalid number of inputs, expected 0, was [%d]", tx.NumInputs())
		assert.Equal(1, tx.NumOutputs(), "invalid number of outputs, expected 1, was [%d]", tx.NumInputs())
		iouState := &states.IOU{}
		assert.NoError(tx.GetOutputAt(0, iouState))

		assert.False(iouState.Amount < 5, "invalid amount, expected at least 5, was [%d]", iouState.Amount)
		assert.Equal(2, iouState.Owners().Count(), "invalid state, expected 2 identities, was [%d]", iouState.Owners().Count())
		assert.True(iouState.Owners().Contain(lender), "invalid state, it does not contain lender identity")
		assert.True(command.Ids.Match([]view.Identity{lender, borrower}), "the command does not contain the lender and borrower identities")
		assert.True(iouState.Owners().Match([]view.Identity{lender, borrower}), "the state does not contain the lender and borrower identities")
		assert.NoError(tx.HasBeenEndorsedBy(borrower), "the borrower has not endorsed")
	default:
		return nil, errors.Errorf("invalid command, expected [create], was [%s]", command.Name)
	}

	// The lender is ready to send back the transaction signed
	_, err = context.RunView(state.NewEndorseView(tx))
	assert.NoError(err)

	// Finally, the lender waits that the transaction completes its lifecycle
	return context.RunView(state.NewFinalityView(tx))
}

type UpdateIOUResponderView struct{}

func (i *UpdateIOUResponderView) Call(context view.Context) (interface{}, error) {
	// When the borrower runs the CollectEndorsementsView, at some point, the borrower sends the assembled transaction
	// to the lender. Therefore, the lender waits to receive the transaction.
	tx, err := state.ReceiveTransaction(context)
	assert.NoError(err, "failed receiving transaction")

	// The lender can now inspect the transaction to ensure it is as expected.
	// Here are examples of possible checks

	// Namespaces are properly populated
	assert.Equal(1, len(tx.Namespaces()), "expected only one namespace")
	assert.Equal("iou", tx.Namespaces()[0], "expected the [iou] namespace, got [%s]", tx.Namespaces()[0])

	switch command := tx.Commands().At(0); command.Name {
	case "update":
		// If the update command is attached to the transaction then...

		// One input and one output containing IOU states are expected
		assert.Equal(1, tx.NumInputs(), "invalid number of inputs, expected 1, was %d", tx.NumInputs())
		assert.Equal(1, tx.NumOutputs(), "invalid number of outputs, expected 1, was %d", tx.NumInputs())
		inState := &states.IOU{}
		assert.NoError(tx.GetInputAt(0, inState))
		outState := &states.IOU{}
		assert.NoError(tx.GetOutputAt(0, outState))

		// Additional checks
		// Same IDs
		assert.Equal(inState.LinearID, outState.LinearID, "invalid state id, [%s] != [%s]", inState.LinearID, outState.LinearID)
		// Valid Amount
		assert.False(outState.Amount >= inState.Amount, "invalid amount, [%d] expected to be less or equal [%d]", outState.Amount, inState.Amount)
		// Same owners
		assert.True(inState.Owners().Match(outState.Owners()), "invalid owners, input and output should have the same owners")
		assert.Equal(2, inState.Owners().Count(), "invalid state, expected 2 identities, was [%d]", inState.Owners().Count())
		// Is the lender one of the owners?
		lenderFound := fabric.GetDefaultLocalMembership(context).IsMe(inState.Owners()[0]) != fabric.GetDefaultLocalMembership(context).IsMe(inState.Owners()[1])
		assert.True(lenderFound, "lender identity not found")
		// Did the borrower sign?
		assert.NoError(tx.HasBeenEndorsedBy(inState.Owners().Filter(
			func(identity view.Identity) bool {
				return !fabric.GetDefaultLocalMembership(context).IsMe(identity)
			})...), "the borrower has not endorsed")
	default:
		return nil, errors.Errorf("invalid command, expected [create], was [%s]", command.Name)
	}

	// The lender is ready to send back the transaction signed
	_, err = context.RunView(state.NewEndorseView(tx))
	assert.NoError(err)

	// Finally, the lender waits that the transaction completes its lifecycle
	return context.RunView(state.NewFinalityView(tx))
}
