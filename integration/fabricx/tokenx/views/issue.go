/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/tokenx/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

const TokenxNamespace = "tokenx"

// Issue contains the parameters for issuing new tokens
type Issue struct {
	// TokenType is the type of token to issue (e.g., "USD", "EUR")
	TokenType string `json:"token_type"`

	// Amount is the amount to issue (in smallest units, 8 decimal places)
	Amount uint64 `json:"amount"`

	// Recipient is the FSC node identity of the token recipient
	Recipient view.Identity `json:"recipient"`

	// Approvers is the list of approver identities (from the approver node)
	Approvers []view.Identity `json:"approvers"`
}

// IssueView is executed by the issuer to create new tokens
// Following the fabricx/simple pattern
type IssueView struct {
	Issue
}

func (i *IssueView) Call(ctx view.Context) (interface{}, error) {
	logger.Infof("[IssueView] START: Issuing %d tokens of type %s", i.Amount, i.TokenType)

	// Create the token state
	// Note: CreatedAt is zero for deterministic endorsement across nodes
	token := &states.Token{
		Type:     i.TokenType,
		Amount:   i.Amount,
		Owner:    i.Recipient,
		IssuerID: "issuer", // Simple identifier
	}

	// Create new transaction
	tx, err := state.NewTransaction(ctx)
	if err != nil {
		logger.Errorf("[IssueView] Failed to create transaction: %v", err)
		return nil, err
	}

	// Set namespace
	tx.SetNamespace(TokenxNamespace)

	// Add issue command
	if err = tx.AddCommand("issue"); err != nil {
		logger.Errorf("[IssueView] Failed to add issue command: %v", err)
		return nil, err
	}

	// Add token as output (LinearID will be set automatically)
	// We set it explicitly based on TxID to ensure uniqueness and validity
	token.LinearID = tx.ID()
	if err = tx.AddOutput(token); err != nil {
		logger.Errorf("[IssueView] Failed to add token output: %v", err)
		return nil, err
	}

	// Create transaction record for audit trail
	// Use a prefixed ID to avoid collision with token LinearID
	// Note: Timestamp is zero for deterministic endorsement
	txRecord := &states.TransactionRecord{
		RecordID:  "txr_" + tx.ID(),
		Type:      states.TxTypeIssue,
		TokenType: i.TokenType,
		Amount:    i.Amount,
		To:        i.Recipient,
	}
	if err = tx.AddOutput(txRecord); err != nil {
		logger.Errorf("[IssueView] Failed to add transaction record: %v", err)
		return nil, err
	}

	// Collect endorsements ONLY from approvers (following simple pattern)
	if _, err = ctx.RunView(state.NewCollectEndorsementsView(tx, i.Approvers...)); err != nil {
		logger.Errorf("[IssueView] Failed to collect endorsements: %v", err)
		return nil, err
	}

	// Create a listener to check when the tx is committed
	network, ch, err := fabric.GetDefaultChannel(ctx)
	if err != nil {
		logger.Errorf("[IssueView] Failed to get default channel: %v", err)
		return nil, err
	}

	lm, err := finality.GetListenerManager(ctx, network.Name(), ch.Name())
	if err != nil {
		logger.Errorf("[IssueView] Failed to get listener manager: %v", err)
		return nil, err
	}

	var wg sync.WaitGroup
	wg.Add(1)

	if err = lm.AddFinalityListener(tx.ID(), NewFinalityListener(tx.ID(), fdriver.Valid, &wg)); err != nil {
		logger.Errorf("[IssueView] Failed to add finality listener: %v", err)
		return nil, err
	}

	// Send the approved transaction to the orderer
	if _, err = ctx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, FinalityTimeout)); err != nil {
		logger.Errorf("[IssueView] Failed to submit tx to ordering: %v", err)
		return nil, err
	}

	// Wait for commit via our listener
	wg.Wait()

	logger.Infof("[IssueView] END: Token issued successfully: type=%s, amount=%d, linearID=%s, txID=%s", i.TokenType, i.Amount, token.LinearID, tx.ID())

	return token.LinearID, nil
}

// IssueViewFactory creates IssueView instances
type IssueViewFactory struct{}

func (f *IssueViewFactory) NewView(in []byte) (view.View, error) {
	v := &IssueView{}
	if err := json.Unmarshal(in, &v.Issue); err != nil {
		return nil, err
	}
	return v, nil
}

// IssuerInitView initializes the issuer to process the tokenx namespace
type IssuerInitView struct{}

func (i *IssuerInitView) Call(ctx view.Context) (interface{}, error) {
	_, ch, err := fabric.GetDefaultChannel(ctx)
	if err != nil {
		return nil, err
	}
	if err := ch.Committer().ProcessNamespace(TokenxNamespace); err != nil {
		return nil, err
	}
	logger.Infof("Issuer initialized for namespace: %s", TokenxNamespace)
	return nil, nil
}

// IssuerInitViewFactory creates IssuerInitView instances
type IssuerInitViewFactory struct{}

func (f *IssuerInitViewFactory) NewView(in []byte) (view.View, error) {
	return &IssuerInitView{}, nil
}
