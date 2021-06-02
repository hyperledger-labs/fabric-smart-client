/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package views

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type AcceptAssetView struct{}

func (a *AcceptAssetView) Call(context view.Context) (interface{}, error) {
	// Expect an state transaction
	tx, err := state.ReceiveTransaction(context)
	assert.NoError(err)

	// Check that the transaction is as expected
	assert.Equal(0, tx.Inputs().Count(), "expected zero input, got [%d]", tx.Inputs().Count())
	assert.Equal(1, tx.Outputs().Count(), "expected one output, got [%d]", tx.Outputs().Count())

	asset := &Asset{}
	assert.NoError(tx.Outputs().At(0).State(asset), "failed unmarshalling asset")
	assert.True(asset.Owner.Equal(context.Me()), "expected me to be the owner, got [%s]", asset.Owner)
	//assert.Equal([]byte("Hello World!!!"), asset.PrivateProperties)

	// Accept and send back
	_, err = context.RunView(state.NewAcceptView(tx))
	assert.NoError(err)

	// Wait for finality
	_, err = context.RunView(state.NewFinalityView(tx))
	assert.NoError(err)
	return nil, nil
}
