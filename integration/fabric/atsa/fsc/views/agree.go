/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type AgreeToSell struct {
	Agreement *states.AgreementToSell

	Approver view.Identity
}

type AgreeToSellView struct {
	*AgreeToSell
}

func (a *AgreeToSellView) Call(context view.Context) (interface{}, error) {
	// The asset owner creates a new transaction, and
	tx, err := state.NewAnonymousTransaction(context)
	assert.NoError(err, "failed creating transaction")

	// Sets the namespace where the state should appear, and
	tx.SetNamespace("asset_transfer")

	// Specifies the command this transaction wants to execute.
	// In particular, the asset owner wants to express the willingness to sell a given asset.
	// The approver will use this information to decide how to validate the transaction

	// Let's now retrieve the asset whose agreement to sell must be posted on the ledger.
	// This is needed to retrieve the identity owning that asset.
	asset := &states.Asset{ID: a.Agreement.ID}
	assetID, err := asset.GetLinearID()
	assert.NoError(err, "cannot compute linear state's id")

	assert.NoError(
		state.GetVault(context).GetState("asset_transfer", assetID, asset),
		"failed loading asset [%s]", assetID,
	)

	// Specifies the command this transaction wants to execute.
	// In particular, the asset owner wants to agree to sell the asset.
	// The approver will use this information to decide how to validate the transaction.
	assert.NoError(tx.AddCommand("agreeToSell", asset.Owner), "failed adding issue command")

	// Add the agreement to sell to the transaction
	a.Agreement.Owner = asset.Owner
	assert.NoError(tx.AddOutput(a.Agreement, state.WithHashHiding()), "failed adding output")

	// The asset owner is ready to collect all the required signatures.
	// Namely from the asset owner itself and the approver. In this order.
	// All signatures are required.
	_, err = context.RunView(state.NewCollectEndorsementsView(tx, asset.Owner, a.Approver))
	assert.NoError(err, "failed collecting endorsement")

	// Send to the ordering service and wait for confirmation
	_, err = context.RunView(state.NewOrderingAndFinalityView(tx))
	assert.NoError(err, "failed asking ordering")

	return tx.ID(), nil
}

type AgreeToSellViewFactory struct{}

func (a *AgreeToSellViewFactory) NewView(in []byte) (view.View, error) {
	f := &AgreeToSellView{AgreeToSell: &AgreeToSell{}}
	err := json.Unmarshal(in, f.AgreeToSell)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}

type AgreeToBuy struct {
	Agreement *states.AgreementToBuy

	Approver view.Identity
}

type AgreeToBuyView struct {
	*AgreeToBuy
}

func (a *AgreeToBuyView) Call(context view.Context) (interface{}, error) {
	// Prepare transaction
	tx, err := state.NewAnonymousTransaction(context)
	assert.NoError(err, "failed creating transaction")
	tx.SetNamespace("asset_transfer")
	me := fabric.GetDefaultIdentityProvider(context).DefaultIdentity()
	assert.NoError(tx.AddCommand("agreeToBuy", me), "failed adding issue command")

	a.Agreement.Owner = me
	assert.NoError(tx.AddOutput(a.Agreement, state.WithHashHiding()), "failed adding output")

	_, err = context.RunView(state.NewCollectEndorsementsView(tx, me))
	assert.NoError(err, "failed collecting endorsement")

	_, err = context.RunView(state.NewCollectApprovesView(tx, a.Approver))
	assert.NoError(err, "failed collecting approves")

	// Send to the ordering service and wait for confirmation
	_, err = context.RunView(state.NewOrderingAndFinalityView(tx))
	assert.NoError(err, "failed asking ordering")

	return tx.ID(), nil
}

type AgreeToBuyViewFactory struct{}

func (a *AgreeToBuyViewFactory) NewView(in []byte) (view.View, error) {
	f := &AgreeToBuyView{AgreeToBuy: &AgreeToBuy{}}
	err := json.Unmarshal(in, f.AgreeToBuy)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}
