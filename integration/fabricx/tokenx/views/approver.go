/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/tokenx/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ApproverView validates and approves token transactions
// Following the fabricx/simple pattern
type ApproverView struct{}

func (a *ApproverView) Call(ctx view.Context) (interface{}, error) {
	logger.Infof("[ApproverView] START: Receiving transaction for approval")

	// Receive the transaction
	tx, err := state.ReceiveTransaction(ctx)
	if err != nil {
		logger.Errorf("[ApproverView] Failed to receive transaction: %v", err)
		return nil, err
	}

	// Check that tx has exactly one command
	if tx.Commands().Count() != 1 {
		logger.Errorf("[ApproverView] Command count validation failed: expected 1, got %d", tx.Commands().Count())
		return nil, fmt.Errorf("cmd count is wrong, expected 1 but got %d", tx.Commands().Count())
	}

	cmd := tx.Commands().At(0)

	// Validate based on command type
	switch cmd.Name {
	case "issue":
		if err := a.validateIssue(tx); err != nil {
			logger.Errorf("[ApproverView] Issue validation failed: %v", err)
			return nil, err
		}

	case "transfer":
		if err := a.validateTransfer(tx); err != nil {
			logger.Errorf("[ApproverView] Transfer validation failed: %v", err)
			return nil, err
		}

	case "redeem":
		if err := a.validateRedeem(tx); err != nil {
			logger.Errorf("[ApproverView] Redeem validation failed: %v", err)
			return nil, err
		}

	default:
		logger.Errorf("[ApproverView] Unknown command received: %s", cmd.Name)
		return nil, fmt.Errorf("unknown command: %s", cmd.Name)
	}

	// The approver is ready to send back the transaction signed
	if _, err = ctx.RunView(state.NewEndorseView(tx)); err != nil {
		logger.Errorf("[ApproverView] Failed to endorse transaction: %v", err)
		return nil, err
	}

	logger.Infof("[ApproverView] END: Approved transaction: txID=%s, command=%s", tx.ID(), cmd.Name)

	return nil, nil
}

// validateIssue validates an issue transaction
func (a *ApproverView) validateIssue(tx *state.Transaction) error {
	// Should have 0 inputs (creating new tokens)
	if tx.NumInputs() != 0 {
		return fmt.Errorf("issue should have 0 inputs, got %d", tx.NumInputs())
	}

	// Should have 2 outputs: token + transaction record
	if tx.NumOutputs() != 2 {
		return fmt.Errorf("issue should have 2 outputs, got %d", tx.NumOutputs())
	}

	// Validate the token output
	token := &states.Token{}
	if err := tx.GetOutputAt(0, token); err != nil {
		logger.Errorf("[ApproverView.validateIssue] Failed to get token output at index 0: %v", err)
		return fmt.Errorf("failed getting token output: %w", err)
	}

	// Token must have positive amount
	if token.Amount == 0 {
		logger.Errorf("[ApproverView.validateIssue] Token amount is zero")
		return fmt.Errorf("token amount must be positive")
	}

	// Token must have valid type
	if token.Type == "" {
		logger.Errorf("[ApproverView.validateIssue] Token type is empty")
		return fmt.Errorf("token type cannot be empty")
	}

	return nil
}

// validateTransfer validates a transfer transaction
func (a *ApproverView) validateTransfer(tx *state.Transaction) error {
	// Should have at least 1 input (the token being transferred)
	if tx.NumInputs() < 1 {
		return fmt.Errorf("transfer should have at least 1 input, got %d", tx.NumInputs())
	}

	// Should have at least 2 outputs: token(s) + transaction record
	if tx.NumOutputs() < 2 {
		return fmt.Errorf("transfer should have at least 2 outputs, got %d", tx.NumOutputs())
	}

	// Validate the output token
	token := &states.Token{}
	if err := tx.GetOutputAt(0, token); err != nil {
		logger.Errorf("[ApproverView.validateTransfer] Failed to get token output at index 0: %v", err)
		return fmt.Errorf("failed getting token output: %w", err)
	}

	// Token must have positive amount
	if token.Amount == 0 {
		return fmt.Errorf("transfer output token amount must be positive")
	}

	logger.Infof("[ApproverView.validateTransfer] Transfer validated: type=%s, amount=%d", token.Type, token.Amount)
	return nil
}

// validateRedeem validates a redeem (burn) transaction
func (a *ApproverView) validateRedeem(tx *state.Transaction) error {
	// Should have exactly 1 input (the token being burned)
	if tx.NumInputs() != 1 {
		return fmt.Errorf("redeem should have 1 input, got %d", tx.NumInputs())
	}

	// Should have 2 outputs: delete marker for consumed token + transaction record
	// Note: tx.Delete() creates a "delete" output in the write set
	if tx.NumOutputs() != 2 {
		return fmt.Errorf("redeem should have 2 outputs (delete + tx record), got %d", tx.NumOutputs())
	}

	logger.Infof("[ApproverView.validateRedeem] Redeem validated: inputs=%d, outputs=%d", tx.NumInputs(), tx.NumOutputs())
	return nil
}
