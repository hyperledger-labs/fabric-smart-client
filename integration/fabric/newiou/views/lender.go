/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package views

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type CreateIOUResponderView struct {
	respondExchangeRecipientIdentitiesView *state.RespondExchangeRecipientIdentitiesViewFactory
	receiveTransactionView                 *state.ReceiveTransactionViewFactory
	endorseView                            *endorser.EndorseViewFactory
	finalityView                           *endorser.FinalityViewFactory
	fnsProvider                            *fabric.NetworkServiceProvider
}

func (i *CreateIOUResponderView) Call(context view.Context) (interface{}, error) {
	// As a first step, the lender responds to the request to exchange recipient identities.
	v, err := i.respondExchangeRecipientIdentitiesView.New()
	assert.NoError(err)
	ids, err := context.RunView(v)
	assert.NoError(err, "failed exchanging recipient identities")

	lender, borrower := ids.([]view.Identity)[0], ids.([]view.Identity)[1]

	// When the borrower runs the CollectEndorsementsView, at some point, the borrower sends the assembled transaction
	// to the lender. Therefore, the lender waits to receive the transaction.
	txBoxed, err := context.RunView(i.receiveTransactionView.New(nil), view.WithSameContext())
	assert.NoError(err, "failed receiving transaction")

	tx := txBoxed.(*state.Transaction)

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
		assert.Equal(1, tx.NumOutputs(), "invalid number of outputs, expected 1, was [%d]", tx.NumOutputs())
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
	_, err = context.RunView(i.endorseView.New(tx.Transaction))
	assert.NoError(err)

	// Finally, the lender waits that the transaction completes its lifecycle
	return context.RunView(i.finalityView.NewWithTimeout(tx.Transaction, 1*time.Minute))
}

func NewCreateIOUResponderViewFactory(
	respondExchangeRecipientIdentitiesView *state.RespondExchangeRecipientIdentitiesViewFactory,
	receiveTransactionView *state.ReceiveTransactionViewFactory,
	endorseView *endorser.EndorseViewFactory,
	finalityView *endorser.FinalityViewFactory,
	fnsProvider *fabric.NetworkServiceProvider,
) *CreateIOUResponderViewFactory {
	return &CreateIOUResponderViewFactory{
		respondExchangeRecipientIdentitiesView: respondExchangeRecipientIdentitiesView,
		receiveTransactionView:                 receiveTransactionView,
		endorseView:                            endorseView,
		finalityView:                           finalityView,
		fnsProvider:                            fnsProvider,
	}
}

type CreateIOUResponderViewFactory struct {
	respondExchangeRecipientIdentitiesView *state.RespondExchangeRecipientIdentitiesViewFactory
	receiveTransactionView                 *state.ReceiveTransactionViewFactory
	endorseView                            *endorser.EndorseViewFactory
	finalityView                           *endorser.FinalityViewFactory
	fnsProvider                            *fabric.NetworkServiceProvider
}

func (c *CreateIOUResponderViewFactory) NewView([]byte) (view.View, error) {
	return &CreateIOUResponderView{
		respondExchangeRecipientIdentitiesView: c.respondExchangeRecipientIdentitiesView,
		receiveTransactionView:                 c.receiveTransactionView,
		endorseView:                            c.endorseView,
		finalityView:                           c.finalityView,
		fnsProvider:                            c.fnsProvider,
	}, nil
}

type UpdateIOUResponderView struct {
	receiveTransactionView *state.ReceiveTransactionViewFactory
	endorseView            *endorser.EndorseViewFactory
	finalityView           *endorser.FinalityViewFactory
	fnsProvider            *fabric.NetworkServiceProvider
}

func (i *UpdateIOUResponderView) Call(context view.Context) (interface{}, error) {
	// When the borrower runs the CollectEndorsementsView, at some point, the borrower sends the assembled transaction
	// to the lender. Therefore, the lender waits to receive the transaction.
	txBoxed, err := context.RunView(i.receiveTransactionView.New(nil), view.WithSameContext())
	assert.NoError(err, "failed receiving transaction")

	tx := txBoxed.(*state.Transaction)

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
		fns, err := i.fnsProvider.FabricNetworkService(fabric.DefaultNetwork)
		assert.NoError(err)
		lenderFound := fns.LocalMembership().IsMe(inState.Owners()[0]) != fns.LocalMembership().IsMe(inState.Owners()[1])
		assert.True(lenderFound, "lender identity not found")
		// Did the borrower sign?
		assert.NoError(tx.HasBeenEndorsedBy(inState.Owners().Filter(
			func(identity view.Identity) bool {
				return !fns.LocalMembership().IsMe(identity)
			})...), "the borrower has not endorsed")
	default:
		return nil, errors.Errorf("invalid command, expected [create], was [%s]", command.Name)
	}

	// The lender is ready to send back the transaction signed
	_, err = context.RunView(i.endorseView.New(tx.Transaction))
	assert.NoError(err)

	// Finally, the lender waits that the transaction completes its lifecycle
	return context.RunView(i.finalityView.NewWithTimeout(tx.Transaction, 1*time.Minute))
}

func NewUpdateIOUResponderViewFactory(
	receiveTransactionView *state.ReceiveTransactionViewFactory,
	endorseView *endorser.EndorseViewFactory,
	finalityView *endorser.FinalityViewFactory,
	fnsProvider *fabric.NetworkServiceProvider,
) *UpdateIOUResponderViewFactory {
	return &UpdateIOUResponderViewFactory{
		receiveTransactionView: receiveTransactionView,
		endorseView:            endorseView,
		finalityView:           finalityView,
		fnsProvider:            fnsProvider,
	}
}

type UpdateIOUResponderViewFactory struct {
	receiveTransactionView *state.ReceiveTransactionViewFactory
	endorseView            *endorser.EndorseViewFactory
	finalityView           *endorser.FinalityViewFactory
	fnsProvider            *fabric.NetworkServiceProvider
}

func (c *UpdateIOUResponderViewFactory) NewView([]byte) (view.View, error) {
	return &UpdateIOUResponderView{
		receiveTransactionView: c.receiveTransactionView,
		endorseView:            c.endorseView,
		finalityView:           c.finalityView,
		fnsProvider:            c.fnsProvider,
	}, nil
}
