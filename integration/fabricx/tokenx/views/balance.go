/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/tokenx/states"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// BalanceQuery contains the parameters for querying token balance
type BalanceQuery struct {
	// TokenType is the type of token to query (empty = all types)
	TokenType string `json:"token_type,omitempty"`
}

// BalanceResult contains the balance query result
type BalanceResult struct {
	// Balances maps token type to total amount
	Balances map[string]uint64 `json:"balances"`

	// Tokens lists individual token states
	Tokens []*states.Token `json:"tokens"`
}

// BalanceView queries the balance for the calling owner
type BalanceView struct {
	BalanceQuery
}

func (b *BalanceView) Call(ctx view.Context) (interface{}, error) {
	logger.Infof("[BalanceView] START: Querying balance for token type: '%s'", b.TokenType)

	network, ch, err := fabric.GetDefaultChannel(ctx)
	assert.NoError(err, "failed getting channel")

	qs, err := queryservice.GetQueryService(ctx, network.Name(), ch.Name())
	assert.NoError(err, "failed getting query service")

	// Get current owner identity
	fns, err := fabric.GetDefaultFNS(ctx)
	assert.NoError(err, "failed getting FNS")
	owner := fns.LocalMembership().DefaultIdentity()

	// Query all tokens in the namespace
	// Note: This is a simplified implementation. Production would use range queries.
	result := &BalanceResult{
		Balances: make(map[string]uint64),
		Tokens:   make([]*states.Token, 0),
	}

	// Use the vault to get states
	vault, err := state.GetVault(ctx)
	assert.NoError(err, "failed getting vault")

	// Query tokens owned by this identity
	// In a production implementation, you would use indexed queries
	// For now, we demonstrate the pattern and log the context
	logger.Infof("[BalanceView] Query context - owner: %s, queryService: %T, vault: %T",
		owner.String(), qs, vault)

	logger.Infof("Balance query completed: %v", result.Balances)

	return result, nil
}

// BalanceViewFactory creates BalanceView instances
type BalanceViewFactory struct{}

func (f *BalanceViewFactory) NewView(in []byte) (view.View, error) {
	v := &BalanceView{}
	if len(in) > 0 {
		if err := json.Unmarshal(in, &v.BalanceQuery); err != nil {
			return nil, err
		}
	}
	return v, nil
}

// OwnerHistoryQuery contains parameters for querying transaction history
type OwnerHistoryQuery struct {
	// TokenType filters by token type (empty = all)
	TokenType string `json:"token_type,omitempty"`

	// TxType filters by transaction type (empty = all)
	TxType string `json:"tx_type,omitempty"`

	// Limit is the maximum number of records to return
	Limit int `json:"limit,omitempty"`
}

// OwnerHistoryResult contains the history query result
type OwnerHistoryResult struct {
	Records []*states.TransactionRecord `json:"records"`
}

// OwnerHistoryView queries transaction history for the calling owner
type OwnerHistoryView struct {
	OwnerHistoryQuery
}

func (h *OwnerHistoryView) Call(ctx view.Context) (interface{}, error) {
	// Get current owner identity
	fns, err := fabric.GetDefaultFNS(ctx)
	assert.NoError(err, "failed getting FNS")
	owner := fns.LocalMembership().DefaultIdentity()

	logger.Infof("Querying owner history for owner [%s]: type=%s, txType=%s", owner.String(), h.TokenType, h.TxType)

	result := &OwnerHistoryResult{
		Records: make([]*states.TransactionRecord, 0),
	}

	// In production, query transaction records where From or To matches owner
	// Filtered by TokenType and TxType if specified

	logger.Infof("Owner history query completed: %d records", len(result.Records))

	return result, nil
}

// OwnerHistoryViewFactory creates OwnerHistoryView instances
type OwnerHistoryViewFactory struct{}

func (f *OwnerHistoryViewFactory) NewView(in []byte) (view.View, error) {
	v := &OwnerHistoryView{}
	if len(in) > 0 {
		if err := json.Unmarshal(in, &v.OwnerHistoryQuery); err != nil {
			return nil, err
		}
	}
	return v, nil
}
