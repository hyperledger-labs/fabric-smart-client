/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/tokenx/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Redeem contains the parameters for redeeming (burning) tokens
type Redeem struct {
	// TokenLinearID is the ID of the token to redeem
	TokenLinearID string `json:"token_linear_id"`

	// Amount is the amount to redeem (must equal token amount for full redemption)
	Amount uint64 `json:"amount"`

	// Approver is the identity of the approver (issuer)
	Approver view.Identity `json:"approver"`
}

// RedeemView is executed by the token owner to burn tokens
type RedeemView struct {
	Redeem
}

func (r *RedeemView) Call(ctx view.Context) (interface{}, error) {
	logger.Infof("[RedeemView] START: Redeeming token %s, amount %d", r.TokenLinearID, r.Amount)

	// Create transaction
	tx, err := state.NewTransaction(ctx)
	assert.NoError(err, "failed creating transaction")

	tx.SetNamespace(TokenxNamespace)

	// Load the input token
	inputToken := &states.Token{}
	assert.NoError(tx.AddInputByLinearID(r.TokenLinearID, inputToken, state.WithCertification()), "failed adding input token")

	// For now, require full redemption (amount must match)
	assert.Equal(r.Amount, inputToken.Amount, "partial redemption not supported, must redeem full amount %d", inputToken.Amount)

	// Get owner identity
	fns, err := fabric.GetDefaultFNS(ctx)
	assert.NoError(err, "failed getting FNS")
	owner := fns.LocalMembership().DefaultIdentity()

	// Add redeem command
	assert.NoError(tx.AddCommand("redeem", owner), "failed adding redeem command")

	// Delete the token (mark as burned)
	assert.NoError(tx.Delete(inputToken), "failed deleting token")

	// Create transaction record
	txRecord := &states.TransactionRecord{
		Type:           states.TxTypeRedeem,
		TokenType:      inputToken.Type,
		Amount:         r.Amount,
		From:           owner,
		Timestamp:      time.Now(),
		TokenLinearIDs: []string{r.TokenLinearID},
	}
	assert.NoError(tx.AddOutput(txRecord), "failed adding transaction record")

	// Collect endorsements: owner, then approver
	_, err = ctx.RunView(state.NewCollectEndorsementsView(tx, owner, r.Approver))
	assert.NoError(err, "failed collecting endorsements")

	// Setup finality listener
	var wg sync.WaitGroup
	wg.Add(1)
	_, ch, err := fabric.GetDefaultChannel(ctx)
	assert.NoError(err, "failed getting channel")
	committer := ch.Committer()
	assert.NoError(committer.AddFinalityListener(tx.ID(), NewFinalityListener(tx.ID(), driver.Valid, &wg)), "failed adding listener")

	// Submit to ordering service
	_, err = ctx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, 1*time.Minute))
	assert.NoError(err, "failed ordering transaction")

	wg.Wait()

	logger.Infof("[RedeemView] END: Token redeemed successfully: txID=%s", tx.ID())

	return tx.ID(), nil
}

// RedeemViewFactory creates RedeemView instances
type RedeemViewFactory struct{}

func (f *RedeemViewFactory) NewView(in []byte) (view.View, error) {
	v := &RedeemView{}
	if err := json.Unmarshal(in, &v.Redeem); err != nil {
		return nil, err
	}
	return v, nil
}
