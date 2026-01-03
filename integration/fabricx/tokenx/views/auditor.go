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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// AuditorBalancesQuery contains parameters for auditor balance queries
type AuditorBalancesQuery struct {
	// TokenType filters by token type (empty = all)
	TokenType string `json:"token_type,omitempty"`

	// Note: Auditor cannot see metadata or private properties
}

// AuditorBalancesResult contains aggregated balance information
type AuditorBalancesResult struct {
	// TotalSupply maps token type to total supply
	TotalSupply map[string]uint64 `json:"total_supply"`

	// TokenCount maps token type to number of tokens
	TokenCount map[string]int `json:"token_count"`
}

// AuditorBalancesView queries all token balances (auditor only)
type AuditorBalancesView struct {
	AuditorBalancesQuery
}

func (a *AuditorBalancesView) Call(ctx view.Context) (interface{}, error) {
	logger.Infof("Auditor querying balances: type=%s", a.TokenType)

	result := &AuditorBalancesResult{
		TotalSupply: make(map[string]uint64),
		TokenCount:  make(map[string]int),
	}

	// In production, iterate over all tokens and aggregate
	// Note: Auditor sees only public data (type, amount), not metadata

	logger.Infof("Auditor balance query completed: %v", result.TotalSupply)

	return result, nil
}

// AuditorBalancesViewFactory creates AuditorBalancesView instances
type AuditorBalancesViewFactory struct{}

func (f *AuditorBalancesViewFactory) NewView(in []byte) (view.View, error) {
	v := &AuditorBalancesView{}
	if len(in) > 0 {
		if err := json.Unmarshal(in, &v.AuditorBalancesQuery); err != nil {
			return nil, err
		}
	}
	return v, nil
}

// AuditorHistoryQuery contains parameters for auditor history queries
type AuditorHistoryQuery struct {
	// TokenType filters by token type (empty = all)
	TokenType string `json:"token_type,omitempty"`

	// TxType filters by transaction type (empty = all)
	TxType string `json:"tx_type,omitempty"`

	// Limit is the maximum number of records to return
	Limit int `json:"limit,omitempty"`

	// Note: Auditor cannot see private properties
}

// AuditorHistoryResult contains transaction history for audit
type AuditorHistoryResult struct {
	Records []*states.TransactionRecord `json:"records"`
	Total   int                         `json:"total"`
}

// AuditorHistoryView queries all transaction history (auditor only)
type AuditorHistoryView struct {
	AuditorHistoryQuery
}

func (a *AuditorHistoryView) Call(ctx view.Context) (interface{}, error) {
	logger.Infof("Auditor querying history: type=%s, txType=%s", a.TokenType, a.TxType)

	result := &AuditorHistoryResult{
		Records: make([]*states.TransactionRecord, 0),
		Total:   0,
	}

	// In production, query all transaction records
	// Note: Returns public data only (no metadata/private properties)

	logger.Infof("Auditor history query completed: %d records", result.Total)

	return result, nil
}

// AuditorHistoryViewFactory creates AuditorHistoryView instances
type AuditorHistoryViewFactory struct{}

func (f *AuditorHistoryViewFactory) NewView(in []byte) (view.View, error) {
	v := &AuditorHistoryView{}
	if len(in) > 0 {
		if err := json.Unmarshal(in, &v.AuditorHistoryQuery); err != nil {
			return nil, err
		}
	}
	return v, nil
}

// AuditorInitView initializes the auditor
type AuditorInitView struct{}

func (a *AuditorInitView) Call(ctx view.Context) (interface{}, error) {
	_, ch, err := fabric.GetDefaultChannel(ctx)
	assert.NoError(err, "failed getting default channel")

	// Auditors don't need to process namespace, just observe
	logger.Infof("Auditor initialized for channel: %s", ch.Name())
	return nil, nil
}

// AuditorInitViewFactory creates AuditorInitView instances
type AuditorInitViewFactory struct{}

func (f *AuditorInitViewFactory) NewView(in []byte) (view.View, error) {
	return &AuditorInitView{}, nil
}

// IssuerHistoryQuery contains parameters for issuer history queries
type IssuerHistoryQuery struct {
	// TokenType filters by token type (empty = all)
	TokenType string `json:"token_type,omitempty"`

	// TxType filters by transaction type (issue/redeem only)
	TxType string `json:"tx_type,omitempty"`

	// Limit is the maximum number of records to return
	Limit int `json:"limit,omitempty"`
}

// IssuerHistoryResult contains issuance/redemption history
type IssuerHistoryResult struct {
	Records     []*states.TransactionRecord `json:"records"`
	TotalIssued map[string]uint64           `json:"total_issued"`
	TotalBurned map[string]uint64           `json:"total_burned"`
}

// IssuerHistoryView queries issuance and redemption history
type IssuerHistoryView struct {
	IssuerHistoryQuery
}

func (i *IssuerHistoryView) Call(ctx view.Context) (interface{}, error) {
	logger.Infof("Issuer querying history: type=%s, txType=%s", i.TokenType, i.TxType)

	result := &IssuerHistoryResult{
		Records:     make([]*states.TransactionRecord, 0),
		TotalIssued: make(map[string]uint64),
		TotalBurned: make(map[string]uint64),
	}

	// In production, query transaction records for issue/redeem types

	logger.Infof("Issuer history query completed")

	return result, nil
}

// IssuerHistoryViewFactory creates IssuerHistoryView instances
type IssuerHistoryViewFactory struct{}

func (f *IssuerHistoryViewFactory) NewView(in []byte) (view.View, error) {
	v := &IssuerHistoryView{}
	if len(in) > 0 {
		if err := json.Unmarshal(in, &v.IssuerHistoryQuery); err != nil {
			return nil, err
		}
	}
	return v, nil
}
