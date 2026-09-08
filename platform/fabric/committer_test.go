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

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
)

type mockCommitter struct {
	driver.Committer
	StatusTxID       driver2.TxID
	StatusCallCount  int
	StatusValidation driver.ValidationCode
	StatusDeps       string
	StatusErr        error

	DiscardTxID      driver2.TxID
	DiscardMsg       string
	DiscardCallCount int
	DiscardErr       error

	ProcessNamespaceCount       int
	AddFinalityListenerCount    int
	RemoveFinalityListenerCount int
	AddTransactionFilterCount   int
	CommitTXCount               int
}

func (m *mockCommitter) ProcessNamespace(nss ...driver2.Namespace) error {
	m.ProcessNamespaceCount++
	return nil
}

func (m *mockCommitter) CommitTX(ctx context.Context, txid driver2.TxID, blockNum driver2.BlockNum, txNum driver2.TxNum, env *common.Envelope) error {
	m.CommitTXCount++
	return nil
}

func (m *mockCommitter) Status(ctx context.Context, txID driver2.TxID) (driver.ValidationCode, string, error) {
	m.StatusCallCount++
	m.StatusTxID = txID
	return m.StatusValidation, m.StatusDeps, m.StatusErr
}

func (m *mockCommitter) DiscardTx(ctx context.Context, txID driver2.TxID, message string) error {
	m.DiscardCallCount++
	m.DiscardTxID = txID
	m.DiscardMsg = message
	return m.DiscardErr
}

func (m *mockCommitter) Start(ctx context.Context) error {
	return nil
}

func (m *mockCommitter) AddFinalityListener(txID driver2.TxID, listener driver.FinalityListener) error {
	m.AddFinalityListenerCount++
	return nil
}

func (m *mockCommitter) RemoveFinalityListener(txID driver2.TxID, listener driver.FinalityListener) error {
	m.RemoveFinalityListenerCount++
	return nil
}

func (m *mockCommitter) AddTransactionFilter(tf driver.TransactionFilter) error {
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
