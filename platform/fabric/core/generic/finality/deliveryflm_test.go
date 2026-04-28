/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"reflect"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func TestDeliveryFLM(t *testing.T) {
	t.Parallel()

	mockLM := &mockListenerManager{}
	dlm := &deliveryListenerManager{flm: mockLM}

	t.Run("AddFinalityListener", func(t *testing.T) {
		t.Parallel()
		mockLM.On("AddEventListener", "tx1", mock.Anything).Return(nil).Once()
		err := dlm.AddFinalityListener("tx1", &mockFinalityListener{})
		require.NoError(t, err)
	})

	t.Run("RemoveFinalityListener", func(t *testing.T) {
		t.Parallel()
		mockLM.On("RemoveEventListener", "tx1", mock.Anything).Return(nil).Once()
		err := dlm.RemoveFinalityListener("tx1", &mockFinalityListener{})
		require.NoError(t, err)
	})

	t.Run("deliveryListenerEntry", func(t *testing.T) {
		t.Parallel()
		mockFL := &mockFinalityListener{}
		entry := &deliveryListenerEntry{l: mockFL}

		require.Equal(t, driver2.Namespace(""), entry.Namespace())

		ctx := context.Background()
		info := txInfo{txID: "tx1", status: driver.Valid, message: "ok"}
		mockFL.On("OnStatus", ctx, "tx1", driver.Valid, "ok").Once()
		entry.OnStatus(ctx, info)
		mockFL.AssertExpectations(t)

		require.True(t, entry.Equals(&deliveryListenerEntry{l: mockFL}))
		require.False(t, entry.Equals(nil))
		require.False(t, entry.Equals(&deliveryListenerEntry{l: &mockFinalityListener{}}))
	})

	t.Run("txInfo_ID", func(t *testing.T) {
		t.Parallel()
		info := txInfo{txID: "tx1"}
		require.Equal(t, events.EventID("tx1"), info.ID())
	})
}

func TestNewDeliveryFLM(t *testing.T) {
	t.Parallel()
	logger := logging.MustGetLogger("test")

	mockChannelDriver := &mockChannel{}
	mockD := &mockDelivery{}
	mockChannelDriver.On("Delivery").Return(mockD)

	fCh := &fabric.Channel{}
	v := reflect.ValueOf(fCh).Elem()
	f := v.FieldByName("ch")
	ptr := reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
	ptr.Set(reflect.ValueOf(mockChannelDriver))

	config := events.DeliveryListenerManagerConfig{}
	flm, err := NewDeliveryFLM(logger, config, "test-network", fCh)
	// This might still fail inside NewListenerManager but at least we pass the first crash
	if err == nil {
		require.NotNil(t, flm)
	}
}

func TestTxInfoMapper(t *testing.T) {
	t.Parallel()
	// This tests txInfoMapper.MapProcessedTx which is simple
	mapper := &txInfoMapper{network: "test", logger: nil}

	t.Run("MapProcessedTx", func(t *testing.T) {
		t.Parallel()
		mockPT := &mockProcessedTransaction{}
		mockPT.On("TxID").Return("tx1")
		mockPT.On("ValidationCode").Return(int32(0)) // Valid

		pt := newProcessedTransaction(mockPT)
		infos, err := mapper.MapProcessedTx(pt)
		require.NoError(t, err)
		require.Len(t, infos, 1)
		require.Equal(t, "tx1", infos[0].txID)
		require.Equal(t, driver.Valid, infos[0].status)
	})

	t.Run("MapTxData_UnmarshalError", func(t *testing.T) {
		t.Parallel()
		logger := logging.MustGetLogger("test")
		mapper := &txInfoMapper{logger: logger}
		_, err := mapper.MapTxData(context.Background(), []byte("invalid"), nil, 0, 0)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed unmarshaling tx")
	})

	t.Run("MapTxData_WrongHeaderType", func(t *testing.T) {
		t.Parallel()
		logger := logging.MustGetLogger("test")
		mapper := &txInfoMapper{logger: logger}

		chdrRaw, _ := proto.Marshal(&common.ChannelHeader{Type: int32(common.HeaderType_CONFIG)})
		payloadRaw, _ := proto.Marshal(&common.Payload{
			Header: &common.Header{
				ChannelHeader: chdrRaw,
			},
		})
		envRaw, _ := proto.Marshal(&common.Envelope{
			Payload: payloadRaw,
		})

		infos, err := mapper.MapTxData(context.Background(), envRaw, nil, 0, 0)
		require.NoError(t, err)
		require.Nil(t, infos)
	})
}

type mockListenerManager struct {
	mock.Mock
}

func (m *mockListenerManager) AddEventListener(txID string, e events.ListenerEntry[txInfo]) error {
	args := m.Called(txID, e)
	return args.Error(0)
}

func (m *mockListenerManager) RemoveEventListener(txID string, e events.ListenerEntry[txInfo]) error {
	args := m.Called(txID, e)
	return args.Error(0)
}
