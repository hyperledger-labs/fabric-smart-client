/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type AcceptAssetView struct{}

func (a *AcceptAssetView) Call(context view.Context) (interface{}, error) {
	// As a first step, the owner responds to the request to exchange recipient identities.
	id, err := state.RespondRequestRecipientIdentity(context)
	assert.NoError(err, "failed to respond to identity request")

	// When the borrower runs the CollectEndorsementsView, at some point, the borrower sends the assembled transaction
	// to the owner. Therefore, the owner waits to receive the transaction.
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
	case "issue":
		// If the issue command is attached to the transaction then...

		// Check that the transaction is as expected
		assert.Equal(0, tx.Inputs().Count(), "expected zero input, got [%d]", tx.Inputs().Count())
		assert.Equal(1, tx.Outputs().Count(), "expected one output, got [%d]", tx.Outputs().Count())

		asset := &states.Asset{}
		assert.NoError(tx.Outputs().At(0).State(asset), "failed unmarshalling asset")
		assert.True(asset.Owner.Equal(id), "expected me to be the owner, got [%s]", asset.Owner)
	default:
		return nil, errors.Errorf("invalid command, expected [issue], was [%s]", command.Name)
	}

	// The owner is ready to send back the transaction signed
	_, err = context.RunView(state.NewEndorseView(tx))
	assert.NoError(err)

	// Finally, the owner waits that the transaction completes its lifecycle
	return context.RunView(state.NewFinalityView(tx))
}
