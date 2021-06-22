/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Create contains the input to create an IOU state
type Create struct {
	// Amount the borrower owes the lender
	Amount uint
	// Lender is the identity of the lender's FSC node
	Lender view.Identity
	// Approver is the identity of the approver's FSC node
	Approver view.Identity
}

type CreateIOUView struct {
	Create
}

func (i *CreateIOUView) Call(context view.Context) (interface{}, error) {
	// As a first step operation, the borrower contacts the lender's FSC node
	// to exchange the identities to use to assign ownership of the freshly created IOU state.
	borrower, lender, err := state.ExchangeRecipientIdentities(context, i.Lender)
	assert.NoError(err, "failed exchanging recipient identity")

	// The borrower creates a new transaction
	tx, err := state.NewTransaction(context)
	assert.NoError(err, "failed creating a new transaction")

	// Sets the namespace where the state should be stored
	tx.SetNamespace("iou")

	// Specifies the command this transaction wants to execute.
	// In particular, the borrower wants to create a new IOU state owned by the borrower and the lender
	// The approver will use this information to decide how validate the transaction
	assert.NoError(tx.AddCommand("create", borrower, lender))

	// The borrower prepares the IOU state
	iou := &states.IOU{
		Amount:  i.Amount,
		Parties: []view.Identity{borrower, lender},
	}
	// and add it to the transaction. At this stage, the ID gets set automatically.
	assert.NoError(tx.AddOutput(iou))

	// The borrower is ready to collect all the required signatures.
	// Namely from the borrower itself, the lender, and the approver. In this order.
	// All signatures are required.
	_, err = context.RunView(state.NewCollectEndorsementsView(tx, borrower, lender, i.Approver))
	assert.NoError(err)

	// At this point the borrower can send the transaction to the ordering service and wait for finality.
	_, err = context.RunView(state.NewOrderingAndFinalityView(tx))
	assert.NoError(err)

	// Return the state ID
	return iou.LinearID, nil
}

type CreateIOUViewFactory struct{}

func (c *CreateIOUViewFactory) NewView(in []byte) (view.View, error) {
	f := &CreateIOUView{}
	err := json.Unmarshal(in, &f.Create)
	assert.NoError(err)
	return f, nil
}

type Update struct {
	LinearID string
	Amount   uint
	Approver view.Identity
}

type UpdateIOUView struct {
	Update
}

func (u UpdateIOUView) Call(context view.Context) (interface{}, error) {
	// Create a new transaction
	tx, err := state.NewTransaction(context)
	assert.NoError(err)
	tx.SetNamespace("iou")

	// let's update the IOU on the worldstate
	iouState := &states.IOU{}
	assert.NoError(tx.AddInputByLinearID(u.LinearID, iouState))
	assert.NoError(tx.AddCommand("update", iouState.Owners()...))

	// Modify the amount
	iouState.Amount = u.Amount

	// Append the modified state
	err = tx.AddOutput(iouState)
	assert.NoError(err)

	// Collect signature from the owners of the state and the approver
	_, err = context.RunView(state.NewCollectEndorsementsView(tx, iouState.Owners()[0], iouState.Owners()[1], u.Approver))
	assert.NoError(err)

	// Send to the ordering service and wait for confirmation
	return context.RunView(state.NewOrderingAndFinalityView(tx))
}

type UpdateIOUViewFactory struct{}

func (c *UpdateIOUViewFactory) NewView(in []byte) (view.View, error) {
	f := &UpdateIOUView{}
	err := json.Unmarshal(in, &f.Update)
	assert.NoError(err)
	return f, nil
}
