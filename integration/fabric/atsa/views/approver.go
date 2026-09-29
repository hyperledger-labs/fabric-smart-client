/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ApproverView struct{}

func (*ApproverView) Call(viewCtx view.Context) (any, error) {
	// When a business party runs the CollectEndorsementsView, at some point, this party sends the assembled transaction
	// to the approver. Therefore, the approver waits to receive the transaction.
	tx, err := state.ReceiveTransaction(viewCtx)
	assert.NoError(err, "failed receiving transaction")

	// The approver can now inspect the transaction to ensure it is as expected.
	// Here are examples of possible checks

	// Namespaces are properly populated
	namespaces, err := tx.Namespaces()
	assert.NoError(err, "failed getting namespaces")
	assert.Equal(1, namespaces.Count(), "expected one namespace, got [%d]", namespaces.Count())
	assert.Equal("asset_transfer", namespaces.At(0), "expected 'asset_transfer', got [%s]", namespaces.At(0))

	// Commands are properly populated
	commands, err := tx.Commands()
	assert.NoError(err, "failed getting commands")
	assert.Equal(1, commands.Count(), "expected one command, got [%d]", commands.Count())
	inputs, err := tx.Inputs()
	assert.NoError(err, "failed getting inputs")
	outputs, err := tx.Outputs()
	assert.NoError(err, "failed getting outputs")
	switch cmd := commands.At(0); cmd.Name {
	case "issue":
		assert.Equal(0, inputs.Count(), "expected zero inputs in issue")
		assert.Equal(1, outputs.Count(), "expected one output in issue")

		assert.Equal(2, cmd.Ids.Count(), "expected two identities in issue")
		assert.False(cmd.Ids[0].Equal(cmd.Ids.Others(cmd.Ids[0])[0]), "expected two different identities in issue")
		assert.NoError(tx.HasBeenEndorsedBy(cmd.Ids...), "expected two valid signatures in issue")

		asset := &states.Asset{}
		assert.NoError(outputs.At(0).State(asset))
		assert.True(cmd.Ids.Contain(asset.Owner), "expected asset to contain one of the two command signers")
		// TODO: check asset
	case "agreeToSell":
		assert.Equal(0, inputs.Count(), "expected zero inputs in agreeToSell")
		assert.Equal(1, outputs.Count(), "expected one output in agreeToSell")

		assert.Equal(1, cmd.Ids.Count(), "expected one identity in agreeToSell")
		assert.NoError(tx.HasBeenEndorsedBy(cmd.Ids[0]), "expected a valid signature in agreeToSell")
		assert.True(outputs.At(0).ID().HasPrefix(states.TypeAssetForSale), "expected agreeToSell prefix")

		agreeToSell := &states.AgreementToSell{}
		assert.NoError(outputs.At(0).State(agreeToSell))
		assert.True(cmd.Ids.Contain(agreeToSell.Owner), "expected agree to sell to contain the command signers")
	case "agreeToBuy":
		assert.Equal(0, inputs.Count(), "expected zero inputs in agreeToBuy")
		assert.Equal(1, outputs.Count(), "expected one output in agreeToBuy")

		assert.Equal(1, cmd.Ids.Count(), "expected one identity in agreeToBuy")
		assert.NoError(tx.HasBeenEndorsedBy(cmd.Ids[0]), "expected a valid signature in agreeToBuy")
		assert.True(outputs.At(0).ID().HasPrefix(states.TypeAssetBid), "expected agreeToBuy prefix")

		agreeToBuy := &states.AgreementToBuy{}
		assert.NoError(outputs.At(0).State(agreeToBuy))
		assert.True(cmd.Ids.Contain(agreeToBuy.Owner), "expected agree to buy to contain the command signers")
	case "transfer":
		assert.Equal(3, inputs.Count(), "expected three input in transfer")
		assert.Equal(3, outputs.Count(), "expected three output in transfer")
		assert.Equal(2, outputs.Deleted().Count(), "expected two delete in transfer")

		assert.Equal(1, inputs.IDs().Filter(state.IDHasPrefixFilter(states.TypeAssetForSale)).Count(), "expected one agreeToSell input")
		assert.Equal(1, inputs.IDs().Filter(state.IDHasPrefixFilter(states.TypeAssetBid)).Count(), "expected one agreeToBuy input")

		assert.True(outputs.IDs().Match(inputs.IDs()))

		assetIn := &states.Asset{}
		assert.NoError(inputs.Filter(state.InputHasIDPrefixFilter(states.TypeAsset)).At(0).State(assetIn))
		assetOut := &states.Asset{}
		assert.NoError(outputs.Written().At(0).State(assetOut))

		assert.Equal(2, cmd.Ids.Count(), "expected two identities in transfer")
		assert.NoError(tx.HasBeenEndorsedBy(cmd.Ids...), "expected two valid signatures in transfer")
		assert.True(cmd.Ids.Match(state.Identities{assetIn.Owner, assetOut.Owner}), "expected asset owners")

		// assert.EqualMod(assetIn, assetOut, []string{"Owner"}, "assets do not match")
	default:
		assert.Fail("expected a valid command, got [%s]", cmd)
	}

	// The approver is ready to send back the transaction signed
	_, err = viewCtx.RunView(state.NewEndorseView(tx))
	assert.NoError(err)

	// Finally, the approver waits that the transaction completes its lifecycle
	return viewCtx.RunView(state.NewFinalityView(tx))
}
