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

// Transfer contains the parameters for transferring tokens
type Transfer struct {
	// TokenLinearID is the ID of the token to transfer
	TokenLinearID string `json:"token_linear_id"`

	// Amount is the amount to transfer (if less than token amount, splits the token)
	Amount uint64 `json:"amount"`

	// Recipient is the FSC node identity of the new owner
	Recipient view.Identity `json:"recipient"`

	// Approver is the identity of the approver (issuer)
	Approver view.Identity `json:"approver"`
}

// TransferView is executed by the current owner to transfer tokens
type TransferView struct {
	Transfer
}

func (t *TransferView) Call(ctx view.Context) (interface{}, error) {
	logger.Infof("[TransferView] START: Transferring token %s, amount %d", t.TokenLinearID, t.Amount)

	// Exchange identities with recipient
	sender, newOwner, err := state.ExchangeRecipientIdentities(ctx, t.Recipient)
	assert.NoError(err, "failed exchanging recipient identity")

	// Create transaction
	tx, err := state.NewTransaction(ctx)
	assert.NoError(err, "failed creating transaction")

	tx.SetNamespace(TokenxNamespace)

	// Load the input token
	inputToken := &states.Token{}
	assert.NoError(tx.AddInputByLinearID(t.TokenLinearID, inputToken, state.WithCertification()), "failed adding input token")

	// Validate transfer amount
	if t.Amount > inputToken.Amount {
		logger.Errorf("[TransferView] Insufficient balance: token has %d, requested %d", inputToken.Amount, t.Amount)
		return nil, errors.Errorf("insufficient balance: token has %d, requested %d", inputToken.Amount, t.Amount)
	}

	// Check transfer limits
	limit := states.DefaultTransferLimit()
	if t.Amount > limit.MaxAmountPerTx {
		return nil, errors.Errorf("transfer amount %d exceeds max limit %d", t.Amount, limit.MaxAmountPerTx)
	}
	if t.Amount < limit.MinAmount {
		return nil, errors.Errorf("transfer amount %d below minimum %d", t.Amount, limit.MinAmount)
	}

	// Add transfer command
	assert.NoError(tx.AddCommand("transfer", sender, newOwner), "failed adding transfer command")

	var outputTokenIDs []string

	// Create output token for recipient
	outputToken := &states.Token{
		Type:      inputToken.Type,
		Amount:    t.Amount,
		Owner:     newOwner,
		IssuerID:  inputToken.IssuerID,
		CreatedAt: time.Now(),
	}
	assert.NoError(tx.AddOutput(outputToken), "failed adding output token")
	outputTokenIDs = append(outputTokenIDs, outputToken.LinearID)

	// If partial transfer, create change token for sender
	if t.Amount < inputToken.Amount {
		changeAmount := inputToken.Amount - t.Amount
		changeToken := &states.Token{
			Type:      inputToken.Type,
			Amount:    changeAmount,
			Owner:     sender,
			IssuerID:  inputToken.IssuerID,
			CreatedAt: time.Now(),
		}
		assert.NoError(tx.AddOutput(changeToken), "failed adding change token")
		outputTokenIDs = append(outputTokenIDs, changeToken.LinearID)
	}

	// Create transaction record
	txRecord := &states.TransactionRecord{
		Type:           states.TxTypeTransfer,
		TokenType:      inputToken.Type,
		Amount:         t.Amount,
		From:           sender,
		To:             newOwner,
		Timestamp:      time.Now(),
		TokenLinearIDs: outputTokenIDs,
	}
	assert.NoError(tx.AddOutput(txRecord), "failed adding transaction record")

	// Collect endorsements: sender, recipient, then approver
	_, err = ctx.RunView(state.NewCollectEndorsementsView(tx, sender, newOwner, t.Approver))
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

	logger.Infof("[TransferView] END: Transfer completed: txID=%s", tx.ID())

	return tx.ID(), nil
}

// TransferViewFactory creates TransferView instances
type TransferViewFactory struct{}

func (f *TransferViewFactory) NewView(in []byte) (view.View, error) {
	v := &TransferView{}
	if err := json.Unmarshal(in, &v.Transfer); err != nil {
		return nil, err
	}
	return v, nil
}

// TransferResponderView is executed by the recipient to validate and accept a transfer
type TransferResponderView struct{}

func (t *TransferResponderView) Call(ctx view.Context) (interface{}, error) {
	// Respond with recipient identity
	_, recipientID, err := state.RespondExchangeRecipientIdentities(ctx)
	assert.NoError(err, "failed responding with recipient identity")

	// Receive the transaction
	tx, err := state.ReceiveTransaction(ctx)
	assert.NoError(err, "failed receiving transaction")

	// Validate the transaction
	assert.Equal(1, tx.Commands().Count(), "expected single command")
	cmd := tx.Commands().At(0)
	assert.Equal("transfer", cmd.Name, "expected transfer command, got %s", cmd.Name)

	// Verify we have at least 1 input and 2 outputs (token + record)
	assert.True(tx.NumInputs() >= 1, "expected at least 1 input")
	assert.True(tx.NumOutputs() >= 2, "expected at least 2 outputs")

	// Verify the output token is ours
	outputToken := &states.Token{}
	assert.NoError(tx.GetOutputAt(0, outputToken), "failed getting output token")
	assert.True(outputToken.Owner.Equal(recipientID), "output token owner should be recipient")
	assert.True(outputToken.Amount > 0, "token amount must be positive")

	// Sign the transaction
	_, err = ctx.RunView(state.NewEndorseView(tx))
	assert.NoError(err, "failed endorsing transaction")

	logger.Infof("TransferResponderView: accepted transfer of %d %s tokens", outputToken.Amount, outputToken.Type)

	// Wait for finality
	return ctx.RunView(state.NewFinalityWithTimeoutView(tx, 1*time.Minute))
}
