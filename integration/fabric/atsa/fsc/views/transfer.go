/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/states"
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
	assetOwner, err := state.RequestRecipientIdentity(context, f.Recipient)
	assert.NoError(err, "failed getting recipient identity")

	// Prepare transaction
	tx, err := state.NewAnonymousTransaction(context)
	assert.NoError(err, "failed creating transaction")
	tx.SetNamespace("asset_transfer")

	agreementToSell := &states.AgreementToSell{}
	assert.NoError(tx.AddInputByLinearID(f.AgreementId, agreementToSell, state.WithCertification()), "failed adding input")
	asset := &states.Asset{ID: f.AssetId}
	assetID, err := asset.GetLinearID()
	assert.NoError(err, "cannot compute linear state's id")

	assert.NoError(tx.AddInputByLinearID(assetID, asset, state.WithCertification()), "failed adding input")
	assert.NoError(tx.AddCommand("transfer", asset.Owner, assetOwner), "failed adding issue command")
	asset.Owner = assetOwner
	assert.NoError(tx.AddOutput(asset), "failed adding output")
	assert.NoError(tx.Delete(agreementToSell), "failed deleting")

	// Send tx and receive the modified transaction
	tx2, err := state.SendAndReceiveTransaction(context, tx, assetOwner)
	assert.NoError(err, "failed sending and received transaction")

	// Check that tx2 is as expected
	assert.Equal(3, tx2.Inputs().Count(), "expected three input, got [%d]", tx2.Inputs().Count())
	assert.Equal(3, tx2.Outputs().Count(), "expected three output, got [%d]", tx2.Outputs().Count())
	assert.Equal(2, tx2.Outputs().Deleted().Count(), "expected two delete, got [%d]", tx2.Outputs().Deleted().Count())

	agreementToBuy := &states.AgreementToBuy{}
	inputState := tx2.Inputs().Filter(state.InputHasIDPrefixFilter(states.TypeAssetBid)).At(0)
	assert.NoError(inputState.VerifyCertification(), "failed certifying agreement to buy")
	assert.NoError(inputState.State(agreementToBuy), "failed unmarshalling agreement to buy")

	_, err = context.RunView(state.NewCollectEndorsementsView(tx2, fabric.GetDefaultIdentityProvider(context).DefaultIdentity(), assetOwner))
	assert.NoError(err, "failed collecting endorsement")

	_, err = context.RunView(state.NewCollectApprovesView(tx2, f.Approver))
	assert.NoError(err, "failed collecting approves")

	// Send to the ordering service and wait for confirmation
	_, err = context.RunView(state.NewOrderingAndFinalityView(tx2))
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
	// First, respond to a request for an identity
	id, err := state.RespondRequestRecipientIdentity(context)
	assert.NoError(err, "failed to respond to identity request")

	// Expect an state transaction
	tx, err := state.ReceiveTransaction(context)
	assert.NoError(err)

	// The owner can now inspect the transaction to ensure it is as expected.
	// Here are examples of possible checks

	// Namespaces are properly populated
	assert.Equal(1, len(tx.Namespaces()), "expected only one namespace")
	assert.Equal("asset_transfer", tx.Namespaces()[0], "expected the [asset_transfer] namespace, got [%s]", tx.Namespaces()[0])

	// Commands are properly populated
	assert.Equal(1, tx.Commands().Count(), "expected only a single command, got [%s]", tx.Commands().Count())
	switch command := tx.Commands().At(0); command.Name {
	case "transfer":
		// If the transfer command is attached to the transaction then...

		// Check that the transaction is as expected
		assert.Equal(2, tx.Inputs().Count(), "expected two inputs, got [%d]", tx.Inputs().Count())
		assert.Equal(2, tx.Outputs().Count(), "expected two outputs, got [%d]", tx.Outputs().Count())
		assert.Equal(1, tx.Outputs().Deleted().Count(), "expected one delete, got [%d]", tx.Outputs().Deleted().Count())

		agreementToSell := &states.AgreementToSell{}
		inputState := tx.Inputs().Filter(state.InputHasIDPrefixFilter(states.TypeAssetForSale)).At(0)
		assert.NoError(inputState.VerifyCertification(), "failed certifying agreement to sell")
		assert.NoError(inputState.State(agreementToSell), "failed unmarshalling agreement to sell")

		assetIn := &states.Asset{}
		inputState = tx.Inputs().Filter(state.InputHasIDPrefixFilter(states.TypeAsset)).At(0)
		assert.NoError(inputState.VerifyCertification(), "failed certifying asset in")
		assert.NoError(inputState.State(assetIn), "failed unmarshalling asset in")

		assetOut := &states.Asset{}
		assert.NoError(tx.Outputs().Written().At(0).State(assetOut), "failed unmarshalling asset out")
		assert.True(assetOut.Owner.Equal(id), "expected me to be the owner, got [%s]", assetOut.Owner)

		assert.Equal(assetIn.PrivateProperties, assetOut.PrivateProperties)
		assert.Equal([]byte("Hello World!!!"), assetOut.PrivateProperties)

		// Append agreement to buy
		agreementToBuy := &states.AgreementToBuy{
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
	default:
		return nil, errors.Errorf("invalid command, expected [transfer], was [%s]", command.Name)
	}

	tx2, err := state.SendBackAndReceiveTransaction(context, tx)
	assert.NoError(err, "failed sending back transaction and receiving the new one")

	// TODO: check that tx is equal to tx2

	// The approver is ready to send back the transaction signed
	_, err = context.RunView(state.NewEndorseView(tx2))
	assert.NoError(err)

	// Finally, the approver waits that the transaction completes its lifecycle
	return context.RunView(state.NewFinalityView(tx2))
}
