/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type ApproverView struct{}

func (i *ApproverView) Call(context view.Context) (interface{}, error) {
	// When the borrower runs the CollectEndorsementsView, at some point, the borrower sends the assembled transaction
	// to the approver. Therefore, the approver waits to receive the transaction.
	tx, err := state.ReceiveTransaction(context)
	assert.NoError(err, "failed receiving transaction")

	// The approver can now inspect the transaction to ensure it is as expected.
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
		assert.Equal(1, tx.NumOutputs(), "invalid number of outputs, expected 1, was [%d]", tx.NumOutputs())
		iouState := &states.IOU{}
		assert.NoError(tx.GetOutputAt(0, iouState))

		assert.True(iouState.Amount >= 5, "invalid amount, expected at least 5, was [%d]", iouState.Amount)
		assert.Equal(2, iouState.Owners().Count(), "invalid state, expected 2 identities, was [%d]", iouState.Owners().Count())
		assert.False(iouState.Owners()[0].Equal(iouState.Owners()[1]), "owner identities must be different")
		assert.True(iouState.Owners().Match(command.Ids), "invalid state, it does not contain command's identities")
		assert.NoError(tx.HasBeenEndorsedBy(iouState.Owners()...), "signatures are missing")
	case "update":
		// If the update command is attached to the transaction then...

		// The single input and output should be an IOU state
		assert.Equal(1, tx.NumInputs(), "invalid number of inputs, expected 1, was [%d]", tx.NumInputs())
		assert.Equal(1, tx.NumOutputs(), "invalid number of outputs,  expected 1, was [%d]", tx.NumOutputs())

		inState := &states.IOU{}
		assert.NoError(tx.GetInputAt(0, inState))
		outState := &states.IOU{}
		assert.NoError(tx.GetOutputAt(0, outState))

		assert.Equal(inState.LinearID, outState.LinearID, "invalid state id, [%s] != [%s]", inState.LinearID, outState.LinearID)
		assert.True(outState.Amount < inState.Amount, "invalid amount, [%d] expected to be less or equal [%d]", outState.Amount, inState.Amount)
		assert.True(inState.Owners().Match(outState.Owners()), "invalid owners, input and output should have the same owners")
		assert.NoError(tx.HasBeenEndorsedBy(outState.Owners()...), "signatures are missing")
	default:
		return nil, errors.Errorf("invalid command, expected [create] or [update], was [%s]", command.Name)
	}

	// The approver is ready to send back the transaction signed
	_, err = context.RunView(state.NewEndorseView(tx))
	assert.NoError(err)

	// Check committer events
	var wg sync.WaitGroup
	wg.Add(1)
	_, ch, err := fabric.GetDefaultChannel(context)
	assert.NoError(err)
	committer := ch.Committer()
	assert.NoError(err, committer.AddFinalityListener(tx.ID(), NewFinalityListener(tx.ID(), driver.Valid, &wg)), "failed to add committer listener")
	assert.Error(committer.AddFinalityListener("", NewFinalityListener(tx.ID(), driver.Valid, &wg)), "must have failed")

	// Finally, the approver waits that the transaction completes its lifecycle
	_, err = context.RunView(state.NewFinalityWithTimeoutView(tx, 1*time.Minute))
	assert.NoError(err, "failed to run finality view")
	wg.Wait()

	wg = sync.WaitGroup{}
	wg.Add(1)
	assert.NoError(err, committer.AddFinalityListener(tx.ID(), NewFinalityListener(tx.ID(), driver.Valid, &wg)), "failed to add committer listener")
	wg.Wait()

	return nil, nil
}

type ApproverInitView struct{}

func (a *ApproverInitView) Call(context view.Context) (interface{}, error) {
	_, ch, err := fabric.GetDefaultChannel(context)
	assert.NoError(err)
	assert.NoError(ch.Committer().ProcessNamespace("iou"), "failed to setup namespace to process")
	return nil, nil
}

type ApproverInitViewFactory struct{}

func (c *ApproverInitViewFactory) NewView(in []byte) (view.View, error) {
	f := &ApproverInitView{}
	return f, nil
}
