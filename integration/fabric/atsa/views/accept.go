/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type AcceptAssetView struct{}

func (*AcceptAssetView) Call(viewCtx view.Context) (any, error) {
	// As a first step, the owner responds to the request to exchange recipient identities.
	id, err := state.RespondRequestRecipientIdentity(viewCtx)
	assert.NoError(err, "failed to respond to identity request")

	// When the borrower runs the CollectEndorsementsView, at some point, the borrower sends the assembled transaction
	// to the owner. Therefore, the owner waits to receive the transaction.
	tx, err := state.ReceiveTransaction(viewCtx)
	assert.NoError(err)

	// The owner can now inspect the transaction to ensure it is as expected.
	// Here are examples of possible checks

	// Namespaces are properly populated
	namespaces, err := tx.Namespaces()
	assert.NoError(err, "failed getting namespaces")
	assert.Equal(1, len(namespaces), "expected only one namespace")
	assert.Equal("asset_transfer", namespaces[0], "expected the [asset_transfer] namespace, got [%s]", namespaces[0])

	// Commands are properly populated
	commands, err := tx.Commands()
	assert.NoError(err, "failed getting commands")
	assert.Equal(1, commands.Count(), "expected only a single command, got [%s]", commands.Count())
	switch command := commands.At(0); command.Name {
	case "issue":
		// If the issue command is attached to the transaction then...

		// Check that the transaction is as expected
		inputs, err := tx.Inputs()
		assert.NoError(err, "failed getting inputs")
		outputs, err := tx.Outputs()
		assert.NoError(err, "failed getting outputs")
		assert.Equal(0, inputs.Count(), "expected zero input, got [%d]", inputs.Count())
		assert.Equal(1, outputs.Count(), "expected one output, got [%d]", outputs.Count())

		asset := &states.Asset{}
		assert.NoError(outputs.At(0).State(asset), "failed unmarshalling asset")
		assert.True(asset.Owner.Equal(id), "expected me to be the owner, got [%s]", asset.Owner)
	default:
		return nil, errors.Errorf("invalid command, expected [issue], was [%s]", command.Name)
	}

	// The owner is ready to send back the transaction signed
	_, err = viewCtx.RunView(state.NewEndorseView(tx))
	assert.NoError(err)

	// Finally, the owner waits that the transaction completes its lifecycle
	return viewCtx.RunView(state.NewFinalityView(tx))
}
