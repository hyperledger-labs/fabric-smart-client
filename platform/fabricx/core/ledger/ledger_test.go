/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger/mock"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLedger_GetLedgerInfo(t *testing.T) {
	t.Parallel()
	fakeBlockClient := &mock.BlockQueryServiceClient{}
	fakeQueryService := &mock.QueryService{}
	ctx := context.Background()
	l := ledger.New(fakeBlockClient, fakeQueryService, ctx)

	expectedInfo := &cb.BlockchainInfo{
		Height:            10,
		CurrentBlockHash:  []byte("current"),
		PreviousBlockHash: []byte("previous"),
	}
	fakeBlockClient.GetBlockchainInfoReturns(expectedInfo, nil)

	info, err := l.GetLedgerInfo()
	require.NoError(t, err)
	require.Equal(t, expectedInfo.Height, info.Height)
	require.Equal(t, expectedInfo.CurrentBlockHash, info.CurrentBlockHash)
	require.Equal(t, expectedInfo.PreviousBlockHash, info.PreviousBlockHash)

	require.Equal(t, 1, fakeBlockClient.GetBlockchainInfoCallCount())
	argCtx, _, _ := fakeBlockClient.GetBlockchainInfoArgsForCall(0)
	require.Equal(t, ctx, argCtx)
}

func TestLedger_GetTransactionByID(t *testing.T) {
	t.Parallel()
	fakeBlockClient := &mock.BlockQueryServiceClient{}
	fakeQueryService := &mock.QueryService{}
	ctx := context.Background()
	l := ledger.New(fakeBlockClient, fakeQueryService, ctx)

	txID := "test-tx"

	// Create a valid envelope for HeaderType_MESSAGE
	chdr := &cb.ChannelHeader{
		Type: int32(cb.HeaderType_MESSAGE),
		TxId: txID,
	}
	chdrRaw, err := protoutil.Marshal(chdr)
	require.NoError(t, err)

	payload := &cb.Payload{
		Header: &cb.Header{
			ChannelHeader: chdrRaw,
		},
		Data: []byte("rwset-data"),
	}
	payloadRaw, err := protoutil.Marshal(payload)
	require.NoError(t, err)

	expectedEnv := &cb.Envelope{Payload: payloadRaw}
	fakeBlockClient.GetTxByIDReturns(expectedEnv, nil)

	expectedStatus := int32(committerpb.Status_COMMITTED)
	fakeQueryService.GetTransactionStatusReturns(expectedStatus, nil)

	pt, err := l.GetTransactionByID(txID)
	require.NoError(t, err)
	require.Equal(t, txID, pt.TxID())
	require.Equal(t, expectedStatus, pt.ValidationCode())
	require.True(t, pt.IsValid())
	require.Equal(t, []byte("rwset-data"), pt.Results())

	require.Equal(t, 1, fakeBlockClient.GetTxByIDCallCount())
	_, argTxID, _ := fakeBlockClient.GetTxByIDArgsForCall(0)
	require.Equal(t, txID, argTxID.TxId)

	require.Equal(t, 1, fakeQueryService.GetTransactionStatusCallCount())
	argStatusQueryTxID := fakeQueryService.GetTransactionStatusArgsForCall(0)
	require.Equal(t, txID, argStatusQueryTxID)
}

func TestLedger_GetBlockNumberByTxID(t *testing.T) {
	t.Parallel()
	fakeBlockClient := &mock.BlockQueryServiceClient{}
	fakeQueryService := &mock.QueryService{}
	ctx := context.Background()
	l := ledger.New(fakeBlockClient, fakeQueryService, ctx)

	txID := "test-tx"
	expectedBlock := &cb.Block{
		Header: &cb.BlockHeader{Number: 5},
	}
	fakeBlockClient.GetBlockByTxIDReturns(expectedBlock, nil)

	blockNum, err := l.GetBlockNumberByTxID(txID)
	require.NoError(t, err)
	require.Equal(t, uint64(5), blockNum)

	require.Equal(t, 1, fakeBlockClient.GetBlockByTxIDCallCount())
}

func TestLedger_GetBlockByNumber(t *testing.T) {
	t.Parallel()
	fakeBlockClient := &mock.BlockQueryServiceClient{}
	fakeQueryService := &mock.QueryService{}
	ctx := context.Background()
	l := ledger.New(fakeBlockClient, fakeQueryService, ctx)

	blockNum := uint64(5)
	expectedBlock := &cb.Block{
		Header: &cb.BlockHeader{Number: blockNum},
	}
	fakeBlockClient.GetBlockByNumberReturns(expectedBlock, nil)

	block, err := l.GetBlockByNumber(blockNum)
	require.NoError(t, err)
	require.NotNil(t, block)

	b, ok := block.(*ledger.Block)
	require.True(t, ok)
	require.Equal(t, blockNum, b.Header.Number)

	require.Equal(t, 1, fakeBlockClient.GetBlockByNumberCallCount())
}

func TestProcessedTransaction_IsValid(t *testing.T) {
	t.Parallel()
	pt := ledger.NewProcessedTransaction("", nil, int32(committerpb.Status_COMMITTED), nil)
	require.True(t, pt.IsValid())

	pt = ledger.NewProcessedTransaction("", nil, int32(committerpb.Status_ABORTED_SIGNATURE_INVALID), nil)
	require.False(t, pt.IsValid())
}

func TestProcessedTransaction_Envelope(t *testing.T) {
	t.Parallel()
	env := []byte("test-payload")
	pt := ledger.NewProcessedTransaction("", nil, int32(committerpb.Status_COMMITTED), env)
	require.Equal(t, env, pt.Envelope())
}

func TestProcessedTransaction_Results(t *testing.T) {
	t.Parallel()
	results := []byte("rwset-data")
	pt := ledger.NewProcessedTransaction("", results, int32(committerpb.Status_COMMITTED), nil)
	require.Equal(t, results, pt.Results())
}

func TestLedger_GetLedgerInfo_Error(t *testing.T) {
	t.Parallel()
	fakeBlockClient := &mock.BlockQueryServiceClient{}
	fakeQueryService := &mock.QueryService{}
	ctx := context.Background()
	l := ledger.New(fakeBlockClient, fakeQueryService, ctx)

	expectedErr := errors.New("blockchain info error")
	fakeBlockClient.GetBlockchainInfoReturns(nil, expectedErr)

	info, err := l.GetLedgerInfo()
	require.Error(t, err)
	require.Nil(t, info)
	require.ErrorContains(t, err, "failed to get blockchain info")
}

func TestLedger_GetTransactionByID_GetTxByIDError(t *testing.T) {
	t.Parallel()
	fakeBlockClient := &mock.BlockQueryServiceClient{}
	fakeQueryService := &mock.QueryService{}
	ctx := context.Background()
	l := ledger.New(fakeBlockClient, fakeQueryService, ctx)

	txID := "test-tx"
	expectedErr := errors.New("tx not found")
	fakeBlockClient.GetTxByIDReturns(nil, expectedErr)

	pt, err := l.GetTransactionByID(txID)
	require.Error(t, err)
	require.Nil(t, pt)
	require.ErrorContains(t, err, "failed to get tx for txID")
}

func TestLedger_GetTransactionByID_GetTransactionStatusError(t *testing.T) {
	t.Parallel()
	fakeBlockClient := &mock.BlockQueryServiceClient{}
	fakeQueryService := &mock.QueryService{}
	ctx := context.Background()
	l := ledger.New(fakeBlockClient, fakeQueryService, ctx)

	txID := "test-tx"

	// Create a valid envelope
	chdr := &cb.ChannelHeader{
		Type: int32(cb.HeaderType_MESSAGE),
		TxId: txID,
	}
	chdrRaw, err := protoutil.Marshal(chdr)
	require.NoError(t, err)

	payload := &cb.Payload{
		Header: &cb.Header{
			ChannelHeader: chdrRaw,
		},
		Data: []byte("rwset-data"),
	}
	payloadRaw, err := protoutil.Marshal(payload)
	require.NoError(t, err)

	expectedEnv := &cb.Envelope{Payload: payloadRaw}
	fakeBlockClient.GetTxByIDReturns(expectedEnv, nil)

	expectedErr := errors.New("status query error")
	fakeQueryService.GetTransactionStatusReturns(0, expectedErr)

	pt, err := l.GetTransactionByID(txID)
	require.Error(t, err)
	require.Nil(t, pt)
	require.ErrorContains(t, err, "failed to get transaction status")
}

func TestLedger_GetTransactionByID_NoStatusReturned(t *testing.T) {
	t.Parallel()
	fakeBlockClient := &mock.BlockQueryServiceClient{}
	fakeQueryService := &mock.QueryService{}
	ctx := context.Background()
	l := ledger.New(fakeBlockClient, fakeQueryService, ctx)

	txID := "test-tx"

	// Create a valid envelope
	chdr := &cb.ChannelHeader{
		Type: int32(cb.HeaderType_MESSAGE),
		TxId: txID,
	}
	chdrRaw, err := protoutil.Marshal(chdr)
	require.NoError(t, err)

	payload := &cb.Payload{
		Header: &cb.Header{
			ChannelHeader: chdrRaw,
		},
		Data: []byte("rwset-data"),
	}
	payloadRaw, err := protoutil.Marshal(payload)
	require.NoError(t, err)

	expectedEnv := &cb.Envelope{Payload: payloadRaw}
	fakeBlockClient.GetTxByIDReturns(expectedEnv, nil)

	// Return error to simulate no status returned
	fakeQueryService.GetTransactionStatusReturns(0, errors.New("no status returned for txID"))

	pt, err := l.GetTransactionByID(txID)
	require.Error(t, err)
	require.Nil(t, pt)
	require.ErrorContains(t, err, "no status returned for txID")
}

func TestLedger_GetTransactionByID_InvalidPayload(t *testing.T) {
	t.Parallel()
	fakeBlockClient := &mock.BlockQueryServiceClient{}
	fakeQueryService := &mock.QueryService{}
	ctx := context.Background()
	l := ledger.New(fakeBlockClient, fakeQueryService, ctx)

	txID := "test-tx"

	// Create an envelope with invalid payload
	expectedEnv := &cb.Envelope{Payload: []byte("invalid-payload")}
	fakeBlockClient.GetTxByIDReturns(expectedEnv, nil)

	fakeQueryService.GetTransactionStatusReturns(int32(committerpb.Status_COMMITTED), nil)

	pt, err := l.GetTransactionByID(txID)
	require.Error(t, err)
	require.Nil(t, pt)
	require.ErrorContains(t, err, "failed to unpack results")
}

func TestLedger_GetTransactionByID_InvalidHeaderType(t *testing.T) {
	t.Parallel()
	fakeBlockClient := &mock.BlockQueryServiceClient{}
	fakeQueryService := &mock.QueryService{}
	ctx := context.Background()
	l := ledger.New(fakeBlockClient, fakeQueryService, ctx)

	txID := "test-tx"

	// Create envelope with wrong header type
	chdr := &cb.ChannelHeader{
		Type: int32(cb.HeaderType_CONFIG),
		TxId: txID,
	}
	chdrRaw, err := protoutil.Marshal(chdr)
	require.NoError(t, err)

	payload := &cb.Payload{
		Header: &cb.Header{
			ChannelHeader: chdrRaw,
		},
		Data: []byte("rwset-data"),
	}
	payloadRaw, err := protoutil.Marshal(payload)
	require.NoError(t, err)

	expectedEnv := &cb.Envelope{Payload: payloadRaw}
	fakeBlockClient.GetTxByIDReturns(expectedEnv, nil)

	fakeQueryService.GetTransactionStatusReturns(int32(committerpb.Status_COMMITTED), nil)

	pt, err := l.GetTransactionByID(txID)
	require.Error(t, err)
	require.Nil(t, pt)
	require.ErrorContains(t, err, "only HeaderType_MESSAGE Transactions are supported")
}

func TestLedger_GetBlockNumberByTxID_Error(t *testing.T) {
	t.Parallel()
	fakeBlockClient := &mock.BlockQueryServiceClient{}
	fakeQueryService := &mock.QueryService{}
	ctx := context.Background()
	l := ledger.New(fakeBlockClient, fakeQueryService, ctx)

	txID := "test-tx"
	expectedErr := errors.New("block not found")
	fakeBlockClient.GetBlockByTxIDReturns(nil, expectedErr)

	blockNum, err := l.GetBlockNumberByTxID(txID)
	require.Error(t, err)
	require.Equal(t, uint64(0), blockNum)
	require.ErrorContains(t, err, "failed to get block for txID")
}

func TestLedger_GetBlockByNumber_Error(t *testing.T) {
	t.Parallel()
	fakeBlockClient := &mock.BlockQueryServiceClient{}
	fakeQueryService := &mock.QueryService{}
	ctx := context.Background()
	l := ledger.New(fakeBlockClient, fakeQueryService, ctx)

	blockNum := uint64(5)
	expectedErr := errors.New("block not found")
	fakeBlockClient.GetBlockByNumberReturns(nil, expectedErr)

	block, err := l.GetBlockByNumber(blockNum)
	require.Error(t, err)
	require.Nil(t, block)
	require.ErrorContains(t, err, "failed to get block by number")
}

func TestBlock_DataAt(t *testing.T) {
	t.Parallel()
	data := [][]byte{
		[]byte("tx1"),
		[]byte("tx2"),
		[]byte("tx3"),
	}
	block := &ledger.Block{
		Block: &cb.Block{
			Data: &cb.BlockData{
				Data: data,
			},
		},
	}

	assert.Equal(t, data[0], block.DataAt(0))
	assert.Equal(t, data[1], block.DataAt(1))
	assert.Equal(t, data[2], block.DataAt(2))
}

func TestBlock_ProcessedTransaction(t *testing.T) {
	t.Parallel()
	txID := "test-tx-id"

	// Create a valid transaction envelope
	chdr := &cb.ChannelHeader{
		Type: int32(cb.HeaderType_MESSAGE),
		TxId: txID,
	}
	chdrRaw, err := protoutil.Marshal(chdr)
	require.NoError(t, err)

	payload := &cb.Payload{
		Header: &cb.Header{
			ChannelHeader: chdrRaw,
		},
		Data: []byte("rwset-data"),
	}
	payloadRaw, err := protoutil.Marshal(payload)
	require.NoError(t, err)

	env := &cb.Envelope{Payload: payloadRaw}
	envRaw, err := protoutil.Marshal(env)
	require.NoError(t, err)

	// Create block with transaction
	validationCode := byte(committerpb.Status_COMMITTED)
	block := &ledger.Block{
		Block: &cb.Block{
			Data: &cb.BlockData{
				Data: [][]byte{envRaw},
			},
			Metadata: &cb.BlockMetadata{
				Metadata: [][]byte{
					nil,
					nil,
					{validationCode},
				},
			},
		},
	}

	pt, err := block.ProcessedTransaction(0)
	require.NoError(t, err)
	require.NotNil(t, pt)
	assert.Equal(t, txID, pt.TxID())
	assert.Equal(t, []byte("rwset-data"), pt.Results())
	assert.Equal(t, int32(validationCode), pt.ValidationCode())
	assert.True(t, pt.IsValid())
}

func TestBlock_ProcessedTransaction_UnmarshalError(t *testing.T) {
	t.Parallel()
	block := &ledger.Block{
		Block: &cb.Block{
			Data: &cb.BlockData{
				Data: [][]byte{[]byte("invalid-tx-data")},
			},
		},
	}

	pt, err := block.ProcessedTransaction(0)
	require.Error(t, err)
	require.Nil(t, pt)
	require.ErrorContains(t, err, "failed to unmarshal tx at index")
}

func TestBlock_ProcessedTransaction_InvalidPayload(t *testing.T) {
	t.Parallel()
	// Create an envelope with invalid payload that will fail unpackResults
	env := &cb.Envelope{Payload: []byte("invalid-payload")}
	envRaw, err := protoutil.Marshal(env)
	require.NoError(t, err)

	block := &ledger.Block{
		Block: &cb.Block{
			Data: &cb.BlockData{
				Data: [][]byte{envRaw},
			},
			Metadata: &cb.BlockMetadata{
				Metadata: [][]byte{
					nil,
					nil,
					{byte(committerpb.Status_COMMITTED)},
				},
			},
		},
	}

	pt, err := block.ProcessedTransaction(0)
	require.Error(t, err)
	require.Nil(t, pt)
	require.ErrorContains(t, err, "failed to unmarshal tx at index [0]")
}

func TestProcessedTransaction_TxID(t *testing.T) {
	t.Parallel()
	txID := "test-tx-id"
	pt := ledger.NewProcessedTransaction(txID, nil, 0, nil)
	assert.Equal(t, txID, pt.TxID())
}

func TestProcessedTransaction_ValidationCode(t *testing.T) {
	t.Parallel()
	code := int32(committerpb.Status_COMMITTED)
	pt := ledger.NewProcessedTransaction("", nil, code, nil)
	assert.Equal(t, code, pt.ValidationCode())
}
