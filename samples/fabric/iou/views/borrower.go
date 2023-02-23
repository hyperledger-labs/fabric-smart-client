/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger-labs/fabric-smart-client/samples/fabric/iou/states"
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
	tracer := tracing.Get(context).GetTracer()
	tracer.StartAt("create-view", time.Now())
	defer tracer.End("create-view")

	// use default identities if not specified
	if i.Lender.IsNone() {
		i.Lender = view2.GetIdentityProvider(context).Identity("lender")
	}
	if i.Approver.IsNone() {
		i.Approver = view2.GetIdentityProvider(context).Identity("approver")
	}

	// As a first step operation, the borrower contacts the lender's FSC node
	// to exchange the identities to use to assign ownership of the freshly created IOU state.
	borrower, lender, err := state.ExchangeRecipientIdentities(context, i.Lender)
	assert.NoError(err, "failed exchanging recipient identity")
	tracer.AddEvent("create-view", "completed identity exchange")
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
	tracer.AddEvent("create-view", "completed Endorsements View")

	// At this point the borrower can send the transaction to the ordering service and wait for finality.
	_, err = context.RunView(state.NewOrderingAndFinalityView(tx))
	assert.NoError(err)
	tracer.AddEvent("create-view", "completed finality View")

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

// Update contains the input to update an IOU state
type Update struct {
	// LinearID is the unique identifier of the IOU state
	LinearID string
	// Amount is the new amount. It should smaller than the current amount
	Amount uint
	// Approver is the identity of the approver's FSC node
	Approver view.Identity
}

type UpdateIOUView struct {
	Update
}

func (u UpdateIOUView) Call(context view.Context) (interface{}, error) {
	// use default identities if not specified
	if u.Approver.IsNone() {
		u.Approver = view2.GetIdentityProvider(context).Identity("approver")
	}

	// The borrower starts by creating a new transaction to update the IOU state
	tx, err := state.NewTransaction(context)
	assert.NoError(err)

	// Sets the namespace where the state is stored
	tx.SetNamespace("iou")

	// To update the state, the borrower, first add a dependency to the IOU state of interest.
	iouState := &states.IOU{}
	assert.NoError(tx.AddInputByLinearID(u.LinearID, iouState))
	// The borrower sets the command to the operation to be performed
	assert.NoError(tx.AddCommand("update", iouState.Owners()...))

	// Then, the borrower updates the amount,
	iouState.Amount = u.Amount

	// and add the modified IOU state as output of the transaction.
	err = tx.AddOutput(iouState)
	assert.NoError(err)

	// The borrower is ready to collect all the required signatures.
	// Namely from the borrower itself, the lender, and the approver. In this order.
	// All signatures are required.
	_, err = context.RunView(state.NewCollectEndorsementsView(tx, iouState.Owners()[0], iouState.Owners()[1], u.Approver))
	assert.NoError(err)

	// At this point the borrower can send the transaction to the ordering service and wait for finality.
	_, err = context.RunView(state.NewOrderingAndFinalityView(tx))
	assert.NoError(err)

	// Return the state ID
	return iouState.LinearID, nil
}

type UpdateIOUViewFactory struct{}

func (c *UpdateIOUViewFactory) NewView(in []byte) (view.View, error) {
	f := &UpdateIOUView{}
	err := json.Unmarshal(in, &f.Update)
	assert.NoError(err)
	return f, nil
}
