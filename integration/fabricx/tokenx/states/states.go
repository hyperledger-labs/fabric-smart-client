/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package states

import (
	"encoding/json"
	"math/big"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	// Decimal precision: 8 decimal places (like satoshis for BTC)
	// Amount 100000000 = 1.00000000 tokens
	DecimalPrecision = 8
	DecimalFactor    = 100000000 // 10^8

	// State type prefixes for composite keys
	TypeToken             = "TKN"
	TypeTransactionRecord = "TXR"
	TypeSwapProposal      = "SWP"

	// Transaction types
	TxTypeIssue    = "issue"
	TxTypeTransfer = "transfer"
	TxTypeRedeem   = "redeem"
	TxTypeSwap     = "swap"
)

// Token represents a fungible token state (UTXO model)
// Each token is a discrete state that can be consumed and created
type Token struct {
	// Type is the token type identifier (e.g., "USD", "EUR", "GOLD")
	Type string `json:"type"`

	// Amount is the token amount in smallest units (8 decimal places)
	// e.g., 100000000 = 1.00000000 tokens
	Amount uint64 `json:"amount"`

	// Owner is the current owner's identity (Idemix for privacy)
	Owner view.Identity `json:"owner"`

	// LinearID is the unique identifier for this token state
	LinearID string `json:"linear_id"`

	// IssuerID identifies who issued this token
	IssuerID string `json:"issuer_id"`

	// CreatedAt is the timestamp when this token was created
	CreatedAt time.Time `json:"created_at"`
}

// GetLinearID returns the composite key for world state storage
func (t *Token) GetLinearID() (string, error) {
	return rwset.CreateCompositeKey(TypeToken, []string{t.LinearID})
}

// SetLinearID sets the linear ID if not already set
func (t *Token) SetLinearID(id string) string {
	if len(t.LinearID) == 0 {
		t.LinearID = id
	}
	return t.LinearID
}

// Owners returns the list of identities owning this state
func (t *Token) Owners() state.Identities {
	return []view.Identity{t.Owner}
}

// AmountFloat returns the amount as a float64 for display
func (t *Token) AmountFloat() float64 {
	return float64(t.Amount) / float64(DecimalFactor)
}

// AmountBigInt returns the amount as big.Int for precise calculations
func (t *Token) AmountBigInt() *big.Int {
	return big.NewInt(int64(t.Amount))
}

// TokenFromFloat creates an amount from a float value
func TokenFromFloat(value float64) uint64 {
	return uint64(value * DecimalFactor)
}

// TransactionRecord stores transaction history for audit trail
type TransactionRecord struct {
	// TxID is the Fabric transaction ID
	TxID string `json:"tx_id"`

	// RecordID is a unique identifier for this record
	RecordID string `json:"record_id"`

	// Type is the transaction type (issue, transfer, redeem, swap)
	Type string `json:"type"`

	// TokenType is the type of token involved
	TokenType string `json:"token_type"`

	// Amount is the transaction amount
	Amount uint64 `json:"amount"`

	// From is the sender identity (empty for issue)
	From view.Identity `json:"from,omitempty"`

	// To is the recipient identity (empty for redeem)
	To view.Identity `json:"to,omitempty"`

	// Timestamp is when the transaction occurred
	Timestamp time.Time `json:"timestamp"`

	// TokenLinearIDs contains the IDs of tokens involved
	TokenLinearIDs []string `json:"token_linear_ids"`
}

// GetLinearID returns the composite key for world state storage
func (r *TransactionRecord) GetLinearID() (string, error) {
	return rwset.CreateCompositeKey(TypeTransactionRecord, []string{r.RecordID})
}

// SetLinearID sets the record ID if not already set
func (r *TransactionRecord) SetLinearID(id string) string {
	if len(r.RecordID) == 0 {
		r.RecordID = id
	}
	return r.RecordID
}

// SwapProposal represents a proposal for atomic token swap
// Extensible design: additional fields can be added for advanced features
type SwapProposal struct {
	// ProposalID is the unique identifier for this proposal
	ProposalID string `json:"proposal_id"`

	// OfferedTokenID is the LinearID of the token being offered
	OfferedTokenID string `json:"offered_token_id"`

	// OfferedType is the type of token being offered
	OfferedType string `json:"offered_type"`

	// OfferedAmount is the amount being offered
	OfferedAmount uint64 `json:"offered_amount"`

	// RequestedType is the type of token wanted in return
	RequestedType string `json:"requested_type"`

	// RequestedAmount is the amount wanted in return
	RequestedAmount uint64 `json:"requested_amount"`

	// Proposer is the identity proposing the swap
	Proposer view.Identity `json:"proposer"`

	// Expiry is when this proposal expires
	Expiry time.Time `json:"expiry"`

	// Status is the proposal status (pending, accepted, cancelled, expired)
	Status string `json:"status"`

	// CreatedAt is when the proposal was created
	CreatedAt time.Time `json:"created_at"`
}

const (
	SwapStatusPending   = "pending"
	SwapStatusAccepted  = "accepted"
	SwapStatusCancelled = "cancelled"
	SwapStatusExpired   = "expired"
)

// GetLinearID returns the composite key for world state storage
func (s *SwapProposal) GetLinearID() (string, error) {
	return rwset.CreateCompositeKey(TypeSwapProposal, []string{s.ProposalID})
}

// SetLinearID sets the proposal ID if not already set
func (s *SwapProposal) SetLinearID(id string) string {
	if len(s.ProposalID) == 0 {
		s.ProposalID = id
	}
	return s.ProposalID
}

// Owners returns the proposer as owner
func (s *SwapProposal) Owners() state.Identities {
	return []view.Identity{s.Proposer}
}

// IsExpired checks if the proposal has expired
func (s *SwapProposal) IsExpired() bool {
	return time.Now().After(s.Expiry)
}

// TransferLimit defines configurable transfer limits
type TransferLimit struct {
	// TokenType is the token type this limit applies to ("*" for all)
	TokenType string `json:"token_type"`

	// MaxAmountPerTx is the maximum amount per single transfer
	MaxAmountPerTx uint64 `json:"max_amount_per_tx"`

	// MaxAmountPerDay is the maximum amount per day (0 = unlimited)
	MaxAmountPerDay uint64 `json:"max_amount_per_day"`

	// MinAmount is the minimum transfer amount
	MinAmount uint64 `json:"min_amount"`
}

// DefaultTransferLimit returns a default limit configuration
func DefaultTransferLimit() *TransferLimit {
	return &TransferLimit{
		TokenType:       "*",
		MaxAmountPerTx:  TokenFromFloat(1000000), // 1 million tokens max per tx
		MaxAmountPerDay: 0,                       // Unlimited per day
		MinAmount:       1,                       // Minimum 0.00000001 tokens
	}
}

// JSON marshalling helpers

// UnmarshalToken deserializes a Token from JSON bytes
func UnmarshalToken(raw []byte) (*Token, error) {
	token := &Token{}
	if err := json.Unmarshal(raw, token); err != nil {
		return nil, err
	}
	return token, nil
}

// UnmarshalTransactionRecord deserializes a TransactionRecord from JSON bytes
func UnmarshalTransactionRecord(raw []byte) (*TransactionRecord, error) {
	record := &TransactionRecord{}
	if err := json.Unmarshal(raw, record); err != nil {
		return nil, err
	}
	return record, nil
}

// UnmarshalSwapProposal deserializes a SwapProposal from JSON bytes
func UnmarshalSwapProposal(raw []byte) (*SwapProposal, error) {
	proposal := &SwapProposal{}
	if err := json.Unmarshal(raw, proposal); err != nil {
		return nil, err
	}
	return proposal, nil
}
