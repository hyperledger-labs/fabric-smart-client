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
	ProcessNamespaceCount       int
	StatusCount                 int
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

func (m *mockCommitter) DiscardTx(ctx context.Context, txid driver2.TxID, reason string) error {
	return nil
}

func (m *mockCommitter) Start(ctx context.Context) error {
	return nil
}

func (m *mockCommitter) Status(ctx context.Context, txID driver2.TxID) (driver.ValidationCode, string, error) {
	m.StatusCount++
	return driver.Valid, "", nil
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

	_, _, err := committer.Status(t.Context(), "txid1")
	require.NoError(t, err)
	require.Equal(t, 1, mc.StatusCount)

	require.NoError(t, committer.AddFinalityListener("txid1", nil))
	require.Equal(t, 1, mc.AddFinalityListenerCount)

	require.NoError(t, committer.RemoveFinalityListener("txid1", nil))
	require.Equal(t, 1, mc.RemoveFinalityListenerCount)

	require.NoError(t, committer.AddTransactionFilter(nil))
	require.Equal(t, 1, mc.AddTransactionFilterCount)
}
