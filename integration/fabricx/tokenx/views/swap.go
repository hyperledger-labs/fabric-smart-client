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
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// SwapPropose contains parameters for proposing a token swap
type SwapPropose struct {
	// OfferedTokenID is the LinearID of the token to offer
	OfferedTokenID string `json:"offered_token_id"`

	// RequestedType is the type of token wanted in return
	RequestedType string `json:"requested_type"`

	// RequestedAmount is the amount wanted in return
	RequestedAmount uint64 `json:"requested_amount"`

	// ExpiryMinutes is how long the proposal is valid (default: 60)
	ExpiryMinutes int `json:"expiry_minutes,omitempty"`
}

// SwapProposeView creates a swap proposal
type SwapProposeView struct {
	SwapPropose
}

func (s *SwapProposeView) Call(ctx view.Context) (interface{}, error) {
	logger.Infof("[SwapProposeView] START: Creating swap proposal: offering %s for %d %s",
		s.OfferedTokenID, s.RequestedAmount, s.RequestedType)

	// Create transaction
	tx, err := state.NewAnonymousTransaction(ctx)
	assert.NoError(err, "failed creating transaction")

	tx.SetNamespace(TokenxNamespace)

	// Load the offered token (just to verify ownership, not consuming it yet)
	offeredToken := &states.Token{}
	assert.NoError(tx.AddInputByLinearID(s.OfferedTokenID, offeredToken, state.WithCertification()), "failed loading offered token")

	// Get proposer identity
	fns, err := fabric.GetDefaultFNS(ctx)
	assert.NoError(err, "failed getting FNS")
	proposer := fns.LocalMembership().DefaultIdentity()

	// Verify ownership
	assert.True(offeredToken.Owner.Equal(proposer), "not the owner of the offered token")

	// Set expiry
	expiryMins := s.ExpiryMinutes
	if expiryMins <= 0 {
		expiryMins = 60 // Default 1 hour
	}

	// Create swap proposal
	proposal := &states.SwapProposal{
		OfferedTokenID:  s.OfferedTokenID,
		OfferedType:     offeredToken.Type,
		OfferedAmount:   offeredToken.Amount,
		RequestedType:   s.RequestedType,
		RequestedAmount: s.RequestedAmount,
		Proposer:        proposer,
		Status:          states.SwapStatusPending,
		Expiry:          time.Now().Add(time.Duration(expiryMins) * time.Minute),
		CreatedAt:       time.Now(),
	}
	proposal.ProposalID = tx.ID()

	// Add command and output
	assert.NoError(tx.AddCommand("swap_propose", proposer), "failed adding command")

	// Re-add the token as output (not consumed yet, just referenced)
	assert.NoError(tx.AddOutput(offeredToken), "failed adding token output")
	assert.NoError(tx.AddOutput(proposal), "failed adding proposal output")

	// Get approver (issuer)
	// In a simplified version, we don't need approval for proposals
	// Just submit directly

	// Setup finality listener
	network, ch, err := fabric.GetDefaultChannel(ctx)
	assert.NoError(err, "failed getting channel")

	lm, err := finality.GetListenerManager(ctx, network.Name(), ch.Name())
	assert.NoError(err, "failed getting listener manager")

	var wg sync.WaitGroup
	wg.Add(1)
	assert.NoError(lm.AddFinalityListener(tx.ID(), NewFinalityListener(tx.ID(), fdriver.Valid, &wg)))

	// Submit
	_, err = ctx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, FinalityTimeout))
	assert.NoError(err, "failed ordering transaction")

	wg.Wait()

	logger.Infof("[SwapProposeView] END: Swap proposal created: proposalID=%s, offered=%s(%d), requested=%s(%d)",
		proposal.ProposalID, proposal.OfferedType, proposal.OfferedAmount, proposal.RequestedType, proposal.RequestedAmount)

	return proposal.ProposalID, nil
}

// SwapProposeViewFactory creates SwapProposeView instances
type SwapProposeViewFactory struct{}

func (f *SwapProposeViewFactory) NewView(in []byte) (view.View, error) {
	v := &SwapProposeView{}
	if err := json.Unmarshal(in, &v.SwapPropose); err != nil {
		return nil, err
	}
	return v, nil
}

// SwapAccept contains parameters for accepting a swap proposal
type SwapAccept struct {
	// ProposalID is the ID of the proposal to accept
	ProposalID string `json:"proposal_id"`

	// OfferedTokenID is the LinearID of the token to give in exchange
	OfferedTokenID string `json:"offered_token_id"`

	// Approver is the identity of the approver (issuer)
	Approver view.Identity `json:"approver"`
}

// SwapAcceptView accepts and executes an atomic swap
type SwapAcceptView struct {
	SwapAccept
}

func (s *SwapAcceptView) Call(ctx view.Context) (interface{}, error) {
	logger.Infof("[SwapAcceptView] START: Accepting swap proposal: %s with token %s", s.ProposalID, s.OfferedTokenID)

	// Create anonymous transaction
	tx, err := state.NewAnonymousTransaction(ctx)
	assert.NoError(err, "failed creating transaction")

	tx.SetNamespace(TokenxNamespace)

	// Load the proposal
	proposal := &states.SwapProposal{}
	assert.NoError(tx.AddInputByLinearID(s.ProposalID, proposal, state.WithCertification()), "failed loading proposal")

	// Verify proposal is still valid
	assert.Equal(states.SwapStatusPending, proposal.Status, "proposal is not pending")
	assert.False(proposal.IsExpired(), "proposal has expired")

	// Load the proposer's token (from the proposal)
	proposerToken := &states.Token{}
	assert.NoError(tx.AddInputByLinearID(proposal.OfferedTokenID, proposerToken, state.WithCertification()), "failed loading proposer token")

	// Load the accepter's token
	accepterToken := &states.Token{}
	assert.NoError(tx.AddInputByLinearID(s.OfferedTokenID, accepterToken, state.WithCertification()), "failed loading accepter token")

	// Verify the tokens match the proposal
	assert.Equal(proposal.RequestedType, accepterToken.Type, "token type mismatch")
	assert.True(accepterToken.Amount >= proposal.RequestedAmount, "insufficient amount")

	// Get accepter identity
	fns, err := fabric.GetDefaultFNS(ctx)
	assert.NoError(err, "failed getting FNS")
	accepter := fns.LocalMembership().DefaultIdentity()

	// Verify ownership
	assert.True(accepterToken.Owner.Equal(accepter), "not the owner of the offered token")

	// Add swap command
	assert.NoError(tx.AddCommand("swap", proposal.Proposer, accepter), "failed adding swap command")

	// Create output tokens (swap ownership)
	// Proposer's token goes to accepter
	proposerTokenOut := &states.Token{
		Type:      proposerToken.Type,
		Amount:    proposerToken.Amount,
		Owner:     accepter,
		IssuerID:  proposerToken.IssuerID,
		CreatedAt: time.Now(),
	}
	assert.NoError(tx.AddOutput(proposerTokenOut), "failed adding proposer token output")

	// Accepter's token goes to proposer
	accepterTokenOut := &states.Token{
		Type:      accepterToken.Type,
		Amount:    proposal.RequestedAmount,
		Owner:     proposal.Proposer,
		IssuerID:  accepterToken.IssuerID,
		CreatedAt: time.Now(),
	}
	assert.NoError(tx.AddOutput(accepterTokenOut), "failed adding accepter token output")

	// Handle change if accepter token was larger than requested
	if accepterToken.Amount > proposal.RequestedAmount {
		changeToken := &states.Token{
			Type:      accepterToken.Type,
			Amount:    accepterToken.Amount - proposal.RequestedAmount,
			Owner:     accepter,
			IssuerID:  accepterToken.IssuerID,
			CreatedAt: time.Now(),
		}
		assert.NoError(tx.AddOutput(changeToken), "failed adding change token")
	}

	// Delete the proposal (mark as accepted)
	assert.NoError(tx.Delete(proposal), "failed deleting proposal")

	// Create transaction record
	txRecord := &states.TransactionRecord{
		Type:      states.TxTypeSwap,
		TokenType: proposerToken.Type + "<->" + accepterToken.Type,
		Amount:    proposerToken.Amount,
		From:      proposal.Proposer,
		To:        accepter,
		Timestamp: time.Now(),
	}
	assert.NoError(tx.AddOutput(txRecord), "failed adding transaction record")

	// Collect endorsements: accepter, proposer, approver
	_, err = ctx.RunView(state.NewCollectEndorsementsView(tx, accepter, proposal.Proposer, s.Approver))
	assert.NoError(err, "failed collecting endorsements")

	// Setup finality listener
	network, ch, err := fabric.GetDefaultChannel(ctx)
	assert.NoError(err, "failed getting channel")

	lm, err := finality.GetListenerManager(ctx, network.Name(), ch.Name())
	assert.NoError(err, "failed getting listener manager")

	var wg sync.WaitGroup
	wg.Add(1)
	assert.NoError(lm.AddFinalityListener(tx.ID(), NewFinalityListener(tx.ID(), fdriver.Valid, &wg)))

	// Submit
	_, err = ctx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, FinalityTimeout))
	assert.NoError(err, "failed ordering transaction")

	wg.Wait()

	logger.Infof("[SwapAcceptView] END: Swap completed: txID=%s, proposalID=%s", tx.ID(), s.ProposalID)

	return tx.ID(), nil
}

// SwapAcceptViewFactory creates SwapAcceptView instances
type SwapAcceptViewFactory struct{}

func (f *SwapAcceptViewFactory) NewView(in []byte) (view.View, error) {
	v := &SwapAcceptView{}
	if err := json.Unmarshal(in, &v.SwapAccept); err != nil {
		return nil, err
	}
	return v, nil
}

// SwapView is a marker type used for responder registration
// The actual swap logic is in SwapAcceptView and SwapProposeView
type SwapView struct {
	SwapAccept
}

func (s *SwapView) Call(ctx view.Context) (interface{}, error) {
	return (&SwapAcceptView{SwapAccept: s.SwapAccept}).Call(ctx)
}

// SwapResponderView handles swap-related requests as a counterparty
type SwapResponderView struct{}

func (s *SwapResponderView) Call(ctx view.Context) (interface{}, error) {
	logger.Infof("[SwapResponderView] START: Responding to swap request")

	// Receive the transaction
	tx, err := state.ReceiveTransaction(ctx)
	assert.NoError(err, "failed receiving transaction")

	// Validate the swap
	assert.True(tx.Commands().Count() >= 1, "expected at least one command")
	cmd := tx.Commands().At(0)
	assert.True(cmd.Name == "swap" || cmd.Name == "swap_propose", "expected swap command, got %s", cmd.Name)

	// Sign the transaction
	_, err = ctx.RunView(state.NewEndorseView(tx))
	assert.NoError(err, "failed endorsing transaction")

	logger.Infof("[SwapResponderView] END: Endorsed swap transaction: txID=%s", tx.ID())

	// Wait for finality
	return ctx.RunView(state.NewFinalityWithTimeoutView(tx, FinalityTimeout))
}
