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
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
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

	// Load the input token from sidecar (remote query service)
	// This allows us to reference tokens that were issued by other nodes
	inputToken := &states.Token{}
	if err := AddRemoteInput(ctx, tx, TokenxNamespace, r.TokenLinearID, inputToken); err != nil {
		logger.Errorf("[RedeemView] Failed to add remote input: %v", err)
		return nil, errors.Wrapf(err, "failed adding remote input token [%s]", r.TokenLinearID)
	}

	// For now, require full redemption (amount must match)
	assert.Equal(r.Amount, inputToken.Amount, "partial redemption not supported, must redeem full amount %d", inputToken.Amount)

	// Get owner identity
	fns, err := fabric.GetDefaultFNS(ctx)
	assert.NoError(err, "failed getting FNS")
	owner := fns.LocalMembership().DefaultIdentity()

	// Add redeem command
	assert.NoError(tx.AddCommand("redeem", owner), "failed adding redeem command")

	// Delete the token (mark as burned)
	// Set LinearID for delete to work properly with remote input
	inputToken.LinearID = r.TokenLinearID
	assert.NoError(tx.Delete(inputToken), "failed deleting token")

	// Create transaction record
	// Use a prefixed ID to avoid collision with token LinearIDs
	// Note: Timestamp is zero for deterministic endorsement
	txRecord := &states.TransactionRecord{
		RecordID:       "txr_" + tx.ID(),
		Type:           states.TxTypeRedeem,
		TokenType:      inputToken.Type,
		Amount:         r.Amount,
		From:           owner,
		TokenLinearIDs: []string{r.TokenLinearID},
	}
	assert.NoError(tx.AddOutput(txRecord), "failed adding transaction record")

	// Validate RWSet keys BEFORE endorsement (RWSet is closed after endorsement)
	if err := ValidateNamespaceRWSetKeys(tx, TokenxNamespace); err != nil {
		return nil, errors.Wrap(err, "invalid RWSet keys")
	}

	// Collect endorsements from approver only (like CBDC pattern)
	_, err = ctx.RunView(state.NewCollectEndorsementsView(tx, r.Approver))
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
