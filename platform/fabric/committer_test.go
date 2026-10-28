/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
)

type mockCommitter struct {
	driver.Committer
	StatusTxID       cdriver.TxID
	StatusCallCount  int
	StatusValidation driver.ValidationCode
	StatusDeps       string
	StatusErr        error

	DiscardTxID      cdriver.TxID
	DiscardMsg       string
	DiscardCallCount int
	DiscardErr       error

	ProcessNamespaceCount       int
	AddFinalityListenerCount    int
	RemoveFinalityListenerCount int
	AddTransactionFilterCount   int
	CommitTXCount               int
}

func (m *mockCommitter) ProcessNamespace(_ ...cdriver.Namespace) error {
	m.ProcessNamespaceCount++
	return nil
}

func (m *mockCommitter) CommitTX(_ context.Context, _ cdriver.TxID, _ cdriver.BlockNum, _ cdriver.TxNum, _ *common.Envelope) error {
	m.CommitTXCount++
	return nil
}

func (m *mockCommitter) Status(_ context.Context, txID cdriver.TxID) (driver.ValidationCode, string, error) {
	m.StatusCallCount++
	m.StatusTxID = txID
	return m.StatusValidation, m.StatusDeps, m.StatusErr
}

func (m *mockCommitter) DiscardTx(_ context.Context, txID cdriver.TxID, message string) error {
	m.DiscardCallCount++
	m.DiscardTxID = txID
	m.DiscardMsg = message
	return m.DiscardErr
}

func (*mockCommitter) Start(_ context.Context) error {
	return nil
}

func (m *mockCommitter) AddFinalityListener(_ cdriver.TxID, _ driver.FinalityListener) error {
	m.AddFinalityListenerCount++
	return nil
}

func (m *mockCommitter) RemoveFinalityListener(_ cdriver.TxID, _ driver.FinalityListener) error {
	m.RemoveFinalityListenerCount++
	return nil
}

func (m *mockCommitter) AddTransactionFilter(_ driver.TransactionFilter) error {
	m.AddTransactionFilterCount++
	return nil
}

func TestCommitter(t *testing.T) {
	t.Parallel()

	mockCh := &mock.Channel{}
	mc := &mockCommitter{}
	mockCh.CommitterReturns(mc)

	committer := NewCommitter(mockCh)

	require.NoError(t, committer.ProcessNamespace("ns1"))
	require.Equal(t, 1, mc.ProcessNamespaceCount)

	mc.StatusValidation = driver.Valid
	mc.StatusDeps = "deps"
	code, msg, err := committer.Status(t.Context(), "txid1")
	require.NoError(t, err)
	require.Equal(t, 1, mc.StatusCallCount)
	require.Equal(t, driver.Valid, code)
	require.Equal(t, "deps", msg)

	require.NoError(t, committer.AddFinalityListener("txid1", nil))
	require.Equal(t, 1, mc.AddFinalityListenerCount)

	require.NoError(t, committer.RemoveFinalityListener("txid1", nil))
	require.Equal(t, 1, mc.RemoveFinalityListenerCount)

	require.NoError(t, committer.AddTransactionFilter(nil))
	require.Equal(t, 1, mc.AddTransactionFilterCount)
}
