/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/events"
	fabricdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func TestDeliveryScanQueryByID(t *testing.T) {
	t.Parallel()

	logger := logging.MustGetLogger("test")

	setup := func() (*DeliveryScanQueryByID[txInfo], *mockEventInfoMapper, *mockDelivery, *fabric.Delivery) {
		mockMapper := &mockEventInfoMapper{}
		mockD := &mockDelivery{}

		fDelivery := &fabric.Delivery{}
		v := reflect.ValueOf(fDelivery).Elem()
		f := v.FieldByName("delivery")
		ptr := reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
		ptr.Set(reflect.ValueOf(mockD))

		q := &DeliveryScanQueryByID[txInfo]{
			Logger:   logger,
			Delivery: fDelivery,
			Mapper:   mockMapper,
		}
		return q, mockMapper, mockD, fDelivery
	}

	t.Run("QueryByID_Success", func(t *testing.T) {
		t.Parallel()
		q, mockMapper, mockD, _ := setup()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		evicted := map[driver.TxID][]events.ListenerEntry[txInfo]{
			"tx1": nil,
		}

		mockD.On("ScanFromBlock", ctx, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			callback := args.Get(2).(fabricdriver.DeliveryCallback)
			mockPT := &mockProcessedTransaction{}
			mockPT.On("TxID").Return("tx1")
			mockPT.On("ValidationCode").Return(int32(0))
			mockPT.On("Results").Return([]byte("results"))

			// Mapper expects *fabric.ProcessedTransaction, so we wrap mockPT
			pt := newProcessedTransaction(mockPT)
			mockMapper.On("MapProcessedTx", pt).Return([]txInfo{{txID: "tx1"}}, nil)

			// Callback (driver.DeliveryCallback) expects driver.ProcessedTransaction, so we pass mockPT
			stop, err := callback(mockPT)
			require.NoError(t, err)
			require.True(t, stop)
		}).Return(nil).Once()

		ch, err := q.QueryByID(ctx, 100, evicted)
		require.NoError(t, err)

		select {
		case infos := <-ch:
			require.Len(t, infos, 1)
			require.Equal(t, "tx1", infos[0].txID)
		case <-ctx.Done():
			t.Fatal("timeout")
		}
	})

	t.Run("QueryByID_MapperError", func(t *testing.T) {
		t.Parallel()
		q, mockMapper, mockD, _ := setup()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		evicted := map[driver.TxID][]events.ListenerEntry[txInfo]{
			"tx1": nil,
		}

		mockD.On("ScanFromBlock", ctx, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			callback := args.Get(2).(fabricdriver.DeliveryCallback)
			mockPT := &mockProcessedTransaction{}
			mockPT.On("TxID").Return("tx1")
			mockPT.On("ValidationCode").Return(int32(0))
			mockPT.On("Results").Return([]byte("results"))

			pt := newProcessedTransaction(mockPT)
			mockMapper.On("MapProcessedTx", pt).Return(nil, errors.New("mapper-error"))

			stop, err := callback(mockPT)
			require.Error(t, err)
			require.True(t, stop)
		}).Return(nil).Once()

		ch, err := q.QueryByID(ctx, 100, evicted)
		require.NoError(t, err)

		select {
		case _, ok := <-ch:
			require.False(t, ok)
		case <-ctx.Done():
			t.Fatal("timeout")
		}
	})

	t.Run("QueryByID_ScanError", func(t *testing.T) {
		t.Parallel()
		q, _, mockD, _ := setup()
		ctx := context.Background()

		evicted := map[driver.TxID][]events.ListenerEntry[txInfo]{
			"tx1": nil,
		}

		mockD.On("ScanFromBlock", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("scan-error")).Once()

		ch, err := q.QueryByID(ctx, 100, evicted)
		require.NoError(t, err)

		select {
		case _, ok := <-ch:
			require.False(t, ok)
		case <-time.After(1 * time.Second):
			t.Fatal("timeout")
		}
	})
	t.Run("QueryByID_PartialSuccess", func(t *testing.T) {
		t.Parallel()
		q, mockMapper, mockD, _ := setup()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		evicted := map[driver.TxID][]events.ListenerEntry[txInfo]{
			"tx1": nil,
			"tx2": nil,
		}

		mockD.On("ScanFromBlock", ctx, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			callback := args.Get(2).(fabricdriver.DeliveryCallback)

			// First TX
			mockPT1 := &mockProcessedTransaction{}
			mockPT1.On("TxID").Return("tx1")
			mockPT1.On("ValidationCode").Return(int32(0))
			mockPT1.On("Results").Return([]byte("results1"))
			pt1 := newProcessedTransaction(mockPT1)
			mockMapper.On("MapProcessedTx", pt1).Return([]txInfo{{txID: "tx1"}}, nil)

			stop, err := callback(mockPT1)
			require.NoError(t, err)
			require.False(t, stop) // Still waiting for tx2

			// Second TX
			mockPT2 := &mockProcessedTransaction{}
			mockPT2.On("TxID").Return("tx2")
			mockPT2.On("ValidationCode").Return(int32(0))
			mockPT2.On("Results").Return([]byte("results2"))
			pt2 := newProcessedTransaction(mockPT2)
			mockMapper.On("MapProcessedTx", pt2).Return([]txInfo{{txID: "tx2"}}, nil)

			stop, err = callback(mockPT2)
			require.NoError(t, err)
			require.True(t, stop) // Done
		}).Return(nil).Once()

		ch, err := q.QueryByID(ctx, 100, evicted)
		require.NoError(t, err)

		resultsCount := 0
		for range ch {
			resultsCount++
			if resultsCount == 2 {
				break
			}
		}
		require.Equal(t, 2, resultsCount)
	})
}

type mockEventInfoMapper struct {
	mock.Mock
}

func (m *mockEventInfoMapper) MapTxData(ctx context.Context, tx []byte, block *common.BlockMetadata, blockNum driver.BlockNum, txNum driver.TxNum) (map[driver.Namespace]txInfo, error) {
	args := m.Called(ctx, tx, block, blockNum, txNum)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[driver.Namespace]txInfo), args.Error(1)
}

func (m *mockEventInfoMapper) MapProcessedTx(tx *fabric.ProcessedTransaction) ([]txInfo, error) {
	args := m.Called(tx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]txInfo), args.Error(1)
}
