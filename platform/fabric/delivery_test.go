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

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
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

func (*mockDelivery) Start(_ context.Context) error {
	return nil
}

func (m *mockDelivery) ScanBlock(_ context.Context, callback driver.BlockCallback) error {
	m.ScanBlockCount++
	if m.TriggerBlockCallback != nil {
		m.TriggerBlockCallback(callback)
	}
	return m.CallbackError
}

func (m *mockDelivery) ScanBlockFrom(_ context.Context, block uint64, callback driver.BlockCallback) error {
	m.ScanBlockFromCount++
	m.LastBlockNum = block
	if m.TriggerBlockCallback != nil {
		m.TriggerBlockCallback(callback)
	}
	return m.CallbackError
}

func (m *mockDelivery) Scan(_ context.Context, txID string, callback driver.DeliveryCallback) error {
	m.ScanCount++
	m.LastTxID = txID
	if m.TriggerDeliveryCallback != nil {
		m.TriggerDeliveryCallback(callback)
	}
	return m.CallbackError
}

func (m *mockDelivery) ScanFromBlock(_ context.Context, block uint64, callback driver.DeliveryCallback) error {
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

	callbackInvoked := false
	// Test ScanBlock
	md.TriggerBlockCallback = func(cb driver.BlockCallback) {
		res, err := cb(ctx, &common.Block{Header: &common.BlockHeader{Number: 10}})
		require.NoError(t, err)
		require.True(t, res)
		callbackInvoked = true
	}
	err := delivery.ScanBlock(ctx, func(_ context.Context, _ *common.Block) (bool, error) {
		return true, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, md.ScanBlockCount)
	require.True(t, callbackInvoked)

	// Test ScanBlockFrom
	callbackInvoked = false
	err = delivery.ScanBlockFrom(ctx, uint64(20), func(_ context.Context, _ *common.Block) (bool, error) {
		callbackInvoked = true
		return true, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, md.ScanBlockFromCount)
	require.Equal(t, uint64(20), md.LastBlockNum)
	require.True(t, callbackInvoked)

	// Test Scan
	callbackInvoked = false
	md.TriggerDeliveryCallback = func(cb driver.DeliveryCallback) {
		res, err := cb(&mockProcessedTx{})
		require.NoError(t, err)
		require.True(t, res)
		callbackInvoked = true
	}
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
	md.CallbackResult = false
	md.CallbackError = errors.New("scan error")

	err = delivery.ScanBlock(ctx, func(_ context.Context, _ *common.Block) (bool, error) {
		return true, nil
	})
	require.ErrorContains(t, err, "scan error")

	err = delivery.ScanBlockFrom(ctx, uint64(20), func(_ context.Context, _ *common.Block) (bool, error) {
		return true, nil
	})
	require.ErrorContains(t, err, "scan error")

	err = delivery.Scan(ctx, "txid1", func(_ *ProcessedTransaction) (bool, error) {
		return true, nil
	})
	require.ErrorContains(t, err, "scan error")

	err = delivery.ScanFromBlock(ctx, uint64(30), func(_ *ProcessedTransaction) (bool, error) {
		return true, nil
	})
	require.ErrorContains(t, err, "scan error")
}
