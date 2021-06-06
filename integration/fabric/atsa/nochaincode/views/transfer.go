/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Transfer struct {
	AgreementId string
	AssetId     string
	Recipient   view.Identity

	Approver view.Identity
}

type TransferView struct {
	*Transfer
}

func (f *TransferView) Call(context view.Context) (interface{}, error) {
	// Prepare transaction
	tx, err := state.NewTransaction(context)
	assert.NoError(err, "failed creating transaction")
	tx.SetNamespace("asset_transfer")

	agreementToSell := &AgreementToSell{}
	assert.NoError(tx.AddInputByLinearID(f.AgreementId, agreementToSell, state.WithCertification()), "failed adding input")
	asset := &Asset{}
	assert.NoError(tx.AddInputByLinearID(f.AssetId, asset, state.WithCertification()), "failed adding input")
	assert.NoError(tx.AddCommand("transfer", asset.Owner, f.Recipient), "failed adding issue command")
	asset.Owner = f.Recipient
	assert.NoError(tx.AddOutput(asset), "failed adding output")
	assert.NoError(tx.Delete(agreementToSell), "failed deleting")

	// Send tx and receive the modified transaction
	tx2, err := state.SendAndReceiveTransaction(context, tx, f.Recipient)
	assert.NoError(err, "failed sending and received transaction")

	// Check that tx2 is as expected
	assert.Equal(3, tx2.Inputs().Count(), "expected three input, got [%d]", tx2.Inputs().Count())
	assert.Equal(3, tx2.Outputs().Count(), "expected three output, got [%d]", tx2.Outputs().Count())
	assert.Equal(2, tx2.Outputs().Deleted().Count(), "expected two delete, got [%d]", tx2.Outputs().Deleted().Count())

	agreementToBuy := &AgreementToBuy{}
	inputState := tx2.Inputs().Filter(state.InputHasIDPrefixFilter(TypeAssetBid)).At(0)
	assert.NoError(inputState.VerifyCertification(), "failed certifying agreement to buy")
	assert.NoError(inputState.State(agreementToBuy), "failed unmarshalling agreement to buy")

	_, err = context.RunView(state.NewCollectEndorsementsView(tx2, fabric.GetIdentityProvider(context).DefaultIdentity(), f.Recipient))
	assert.NoError(err, "failed collecting endorsement")

	_, err = context.RunView(state.NewCollectApprovesView(tx2, f.Approver))
	assert.NoError(err, "failed collecting approves")

	// Send to the ordering service and wait for confirmation
	_, err = context.RunView(state.NewOrderingView(tx2))
	assert.NoError(err, "failed asking ordering")

	return tx.ID(), nil
}

type TransferViewFactory struct{}

func (p *TransferViewFactory) NewView(in []byte) (view.View, error) {
	f := &TransferView{Transfer: &Transfer{}}
	err := json.Unmarshal(in, f.Transfer)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}

type TransferResponderView struct{}

func (t *TransferResponderView) Call(context view.Context) (interface{}, error) {
	// Expect an state transaction
	tx, err := state.ReceiveTransaction(context)
	assert.NoError(err)

	// Check that the transaction is as expected
	assert.Equal(2, tx.Inputs().Count(), "expected two input, got [%d]", tx.Inputs().Count())
	assert.Equal(2, tx.Outputs().Count(), "expected two output, got [%d]", tx.Outputs().Count())
	assert.Equal(1, tx.Outputs().Deleted().Count(), "expected one delete, got [%d]", tx.Outputs().Deleted().Count())

	agreementToSell := &AgreementToSell{}
	inputState := tx.Inputs().Filter(state.InputHasIDPrefixFilter(TypeAssetForSale)).At(0)
	assert.NoError(inputState.VerifyCertification(), "failed certifying agreement to sell")
	assert.NoError(inputState.State(agreementToSell), "failed unmarshalling agreement to sell")

	assetIn := &Asset{}
	inputState = tx.Inputs().Filter(state.InputHasIDPrefixFilter(TypeAsset)).At(0)
	assert.NoError(inputState.VerifyCertification(), "failed certifying asset in")
	assert.NoError(inputState.State(assetIn), "failed unmarshalling asset in")

	assetOut := &Asset{}
	assert.NoError(tx.Outputs().Written().At(0).State(assetOut), "failed unmarshalling asset out")
	assert.True(assetOut.Owner.Equal(fabric.GetIdentityProvider(context).DefaultIdentity()), "expected me to be the owner, got [%s]", assetOut.Owner)

	assert.Equal(assetIn.PrivateProperties, assetOut.PrivateProperties)
	assert.Equal([]byte("Hello World!!!"), assetOut.PrivateProperties)

	// Append agreement to buy
	agreementToBuy := &AgreementToBuy{
		TradeID: agreementToSell.TradeID,
		ID:      agreementToSell.ID,
	}
	agreementToBuyID, err := agreementToBuy.GetLinearID()
	assert.NoError(err, "failed computing agreement to buy linear id")

	// check the agreements
	// Add reference to agreement to buy and delete it
	assert.NoError(tx.AddInputByLinearID(agreementToBuyID, agreementToBuy, state.WithCertification()))
	assert.Equal(agreementToBuy.ID, agreementToSell.ID)
	assert.Equal(agreementToBuy.TradeID, agreementToSell.TradeID)
	assert.Equal(agreementToBuy.Price, agreementToSell.Price)
	assert.NoError(tx.Delete(agreementToBuy))

	tx2, err := state.SendBackAndReceiveTransaction(context, tx)
	assert.NoError(err, "failed sending back transaction and receiving the new one")

	// TODO: check that tx is equal to tx2

	// Accept and send back
	_, err = context.RunView(state.NewEndorseView(tx2))
	assert.NoError(err)

	// Wait for finality
	_, err = context.RunView(state.NewFinalityView(tx2))
	assert.NoError(err)
	return nil, nil
}
