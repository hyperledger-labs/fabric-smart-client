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

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type mockDelivery struct {
	ScanBlockCount          int
	ScanBlockFromCount      int
	ScanCount               int
	ScanFromBlockCount      int
	CallbackResult          bool
	CallbackError           error
	LastBlockNum            uint64
	LastTxID                string
	TriggerBlockCallback    func(callback driver.BlockCallback)
	TriggerDeliveryCallback func(callback driver.DeliveryCallback)
}

func (m *mockDelivery) Start(ctx context.Context) error {
	return nil
}

func (m *mockDelivery) ScanBlock(ctx context.Context, callback driver.BlockCallback) error {
	m.ScanBlockCount++
	if m.TriggerBlockCallback != nil {
		m.TriggerBlockCallback(callback)
	}
	return m.CallbackError
}

func (m *mockDelivery) ScanBlockFrom(ctx context.Context, block uint64, callback driver.BlockCallback) error {
	m.ScanBlockFromCount++
	m.LastBlockNum = block
	if m.TriggerBlockCallback != nil {
		m.TriggerBlockCallback(callback)
	}
	return m.CallbackError
}

func (m *mockDelivery) Scan(ctx context.Context, txID string, callback driver.DeliveryCallback) error {
	m.ScanCount++
	m.LastTxID = txID
	if m.TriggerDeliveryCallback != nil {
		m.TriggerDeliveryCallback(callback)
	}
	return m.CallbackError
}

func (m *mockDelivery) ScanFromBlock(ctx context.Context, block uint64, callback driver.DeliveryCallback) error {
	m.ScanFromBlockCount++
	m.LastBlockNum = block
	if m.TriggerDeliveryCallback != nil {
		m.TriggerDeliveryCallback(callback)
	}
	return m.CallbackError
}

type mockProcessedTx struct {
	driver.ProcessedTransaction
}

func TestDelivery(t *testing.T) {
	t.Parallel()

	md := &mockDelivery{}
	delivery := &Delivery{delivery: md}
	ctx := t.Context()

	// Test ScanBlock
	md.TriggerBlockCallback = func(cb driver.BlockCallback) {
		_, _ = cb(ctx, &common.Block{Header: &common.BlockHeader{Number: 10}})
	}
	callbackInvoked := false
	err := delivery.ScanBlock(ctx, func(ctx context.Context, block *common.Block) (bool, error) {
		callbackInvoked = true
		require.Equal(t, uint64(10), block.Header.Number)
		return true, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, md.ScanBlockCount)
	require.True(t, callbackInvoked)

	// Test ScanBlockFrom
	callbackInvoked = false
	err = delivery.ScanBlockFrom(ctx, uint64(20), func(ctx context.Context, block *common.Block) (bool, error) {
		callbackInvoked = true
		return true, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, md.ScanBlockFromCount)
	require.Equal(t, uint64(20), md.LastBlockNum)
	require.True(t, callbackInvoked)

	// Test Scan
	md.TriggerDeliveryCallback = func(cb driver.DeliveryCallback) {
		_, _ = cb(&mockProcessedTx{})
	}
	callbackInvoked = false
	err = delivery.Scan(ctx, "txid1", func(tx *ProcessedTransaction) (bool, error) {
		callbackInvoked = true
		require.NotNil(t, tx)
		return true, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, md.ScanCount)
	require.Equal(t, "txid1", md.LastTxID)
	require.True(t, callbackInvoked)

	// Test ScanFromBlock
	callbackInvoked = false
	err = delivery.ScanFromBlock(ctx, uint64(30), func(tx *ProcessedTransaction) (bool, error) {
		callbackInvoked = true
		require.NotNil(t, tx)
		return true, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, md.ScanFromBlockCount)
	require.Equal(t, uint64(30), md.LastBlockNum)
	require.True(t, callbackInvoked)
}
