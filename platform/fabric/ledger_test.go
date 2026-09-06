/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type mockBlock struct {
	data [][]byte
}

func (m *mockBlock) DataAt(i int) []byte {
	return m.data[i]
}

func (m *mockBlock) ProcessedTransaction(i int) (driver.ProcessedTransaction, error) {
	return nil, nil
}

type mockLedgerProcessedTx struct {
	driver.ProcessedTransaction
	txID           string
	results        []byte
	validationCode int32
}

func (m *mockLedgerProcessedTx) TxID() string {
	return m.txID
}

func (m *mockLedgerProcessedTx) Results() []byte {
	return m.results
}

func (m *mockLedgerProcessedTx) ValidationCode() int32 {
	return m.validationCode
}

type mockLedger struct {
	mockBlockNum   uint64
	mockBlockErr   error
	mockBlock      *mockBlock
	mockTxErr      error
	mockTx         *mockLedgerProcessedTx
	mockLedgerInfo *driver.LedgerInfo
	mockLedgerErr  error
}

func (m *mockLedger) GetBlockNumberByTxID(txID string) (uint64, error) {
	if m.mockBlockErr != nil {
		return 0, m.mockBlockErr
	}
	return m.mockBlockNum, nil
}

func (m *mockLedger) GetTransactionByID(txID string) (driver.ProcessedTransaction, error) {
	if m.mockTxErr != nil {
		return nil, m.mockTxErr
	}
	return m.mockTx, nil
}

func (m *mockLedger) GetBlockByNumber(number uint64) (driver.Block, error) {
	if m.mockBlockErr != nil {
		return nil, m.mockBlockErr
	}
	return m.mockBlock, nil
}

func (m *mockLedger) GetLedgerInfo() (*driver.LedgerInfo, error) {
	if m.mockLedgerErr != nil {
		return nil, m.mockLedgerErr
	}
	return m.mockLedgerInfo, nil
}

func TestLedger(t *testing.T) {
	t.Parallel()

	ml := &mockLedger{
		mockBlockNum: 10,
		mockBlock:    &mockBlock{data: [][]byte{[]byte("data0"), []byte("data1")}},
		mockTx: &mockLedgerProcessedTx{
			txID:           "tx1",
			results:        []byte("results1"),
			validationCode: 0,
		},
		mockLedgerInfo: &driver.LedgerInfo{},
	}
	ledger := &Ledger{l: ml}

	// Test GetBlockNumberByTxID
	num, err := ledger.GetBlockNumberByTxID("tx1")
	require.NoError(t, err)
	require.Equal(t, uint64(10), num)

	// Test GetTransactionByID
	pt, err := ledger.GetTransactionByID("tx1")
	require.NoError(t, err)
	require.Equal(t, "tx1", pt.TxID())
	require.Equal(t, []byte("results1"), pt.Results())
	require.Equal(t, int32(0), pt.ValidationCode())

	// Test GetTransactionByID error
	ml.mockTxErr = errors.New("tx not found")
	_, err = ledger.GetTransactionByID("tx2")
	require.ErrorContains(t, err, "tx not found")

	// Test GetBlockByNumber
	ml.mockBlockErr = nil
	b, err := ledger.GetBlockByNumber(1)
	require.NoError(t, err)
	require.Equal(t, []byte("data0"), b.DataAt(0))

	// Test GetBlockByNumber error
	ml.mockBlockErr = errors.New("block not found")
	_, err = ledger.GetBlockByNumber(2)
	require.ErrorContains(t, err, "block not found")

	// Test GetLedgerInfo
	info, err := ledger.GetLedgerInfo()
	require.NoError(t, err)
	require.NotNil(t, info)
}
