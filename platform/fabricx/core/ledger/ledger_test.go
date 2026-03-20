/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"context"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger/mock"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"github.com/stretchr/testify/require"
)

func TestLedger_GetLedgerInfo(t *testing.T) {
	fakeBlockClient := &mock.FakeBlockQueryServiceClient{}
	fakeQueryClient := &mock.FakeQueryServiceClient{}
	ctx := context.Background()
	l := New(fakeBlockClient, fakeQueryClient, ctx)

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
	fakeBlockClient := &mock.FakeBlockQueryServiceClient{}
	fakeQueryClient := &mock.FakeQueryServiceClient{}
	ctx := context.Background()
	l := New(fakeBlockClient, fakeQueryClient, ctx)

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

	expectedStatus := committerpb.Status_COMMITTED
	fakeQueryClient.GetTransactionStatusReturns(&committerpb.TxStatusResponse{
		Statuses: []*committerpb.TxStatus{
			{
				Status: expectedStatus,
			},
		},
	}, nil)

	pt, err := l.GetTransactionByID(txID)
	require.NoError(t, err)
	require.Equal(t, txID, pt.TxID())
	require.Equal(t, int32(expectedStatus), pt.ValidationCode())
	require.True(t, pt.IsValid())
	require.Equal(t, []byte("rwset-data"), pt.Results())

	require.Equal(t, 1, fakeBlockClient.GetTxByIDCallCount())
	_, argTxID, _ := fakeBlockClient.GetTxByIDArgsForCall(0)
	require.Equal(t, txID, argTxID.TxId)

	require.Equal(t, 1, fakeQueryClient.GetTransactionStatusCallCount())
	_, argStatusQuery, _ := fakeQueryClient.GetTransactionStatusArgsForCall(0)
	require.Equal(t, txID, argStatusQuery.TxIds[0])
}

func TestLedger_GetBlockNumberByTxID(t *testing.T) {
	fakeBlockClient := &mock.FakeBlockQueryServiceClient{}
	fakeQueryClient := &mock.FakeQueryServiceClient{}
	ctx := context.Background()
	l := New(fakeBlockClient, fakeQueryClient, ctx)

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
	fakeBlockClient := &mock.FakeBlockQueryServiceClient{}
	fakeQueryClient := &mock.FakeQueryServiceClient{}
	ctx := context.Background()
	l := New(fakeBlockClient, fakeQueryClient, ctx)

	blockNum := uint64(5)
	expectedBlock := &cb.Block{
		Header: &cb.BlockHeader{Number: blockNum},
	}
	fakeBlockClient.GetBlockByNumberReturns(expectedBlock, nil)

	block, err := l.GetBlockByNumber(blockNum)
	require.NoError(t, err)
	require.NotNil(t, block)

	b, ok := block.(*Block)
	require.True(t, ok)
	require.Equal(t, blockNum, b.Header.Number)

	require.Equal(t, 1, fakeBlockClient.GetBlockByNumberCallCount())
}

func TestProcessedTransaction_IsValid(t *testing.T) {
	pt := &processedTransaction{
		validationCode: int32(committerpb.Status_COMMITTED),
	}
	require.True(t, pt.IsValid())

	pt.validationCode = int32(committerpb.Status_ABORTED_SIGNATURE_INVALID)
	require.False(t, pt.IsValid())
}

func TestProcessedTransaction_Envelope(t *testing.T) {
	env := []byte("test-payload")
	pt := &processedTransaction{
		envelope: env,
	}
	require.Equal(t, env, pt.Envelope())
}

func TestProcessedTransaction_Results(t *testing.T) {
	results := []byte("rwset-data")
	pt := &processedTransaction{
		results: results,
	}
	require.Equal(t, results, pt.Results())
}
