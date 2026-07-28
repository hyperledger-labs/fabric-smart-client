/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger_test

import (
	"context"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger/mock"
)

// validMessageEnvelope builds a minimal, well-formed HeaderType_MESSAGE
// envelope so that fabricutils.UnmarshalTx/unpackResults succeed, letting a
// test reach the code path under test afterward.
func validMessageEnvelope(t *testing.T, txID string) []byte {
	t.Helper()

	chdr := &cb.ChannelHeader{Type: int32(cb.HeaderType_MESSAGE), TxId: txID}
	chdrRaw, err := protoutil.Marshal(chdr)
	require.NoError(t, err)

	payload := &cb.Payload{
		Header: &cb.Header{ChannelHeader: chdrRaw},
		Data:   []byte("rwset-data"),
	}
	payloadRaw, err := protoutil.Marshal(payload)
	require.NoError(t, err)

	envRaw, err := protoutil.Marshal(&cb.Envelope{Payload: payloadRaw})
	require.NoError(t, err)
	return envRaw
}

// TestGetBlockNumberByTxIDNilHeaderReturnsError proves the fix: a compromised
// or buggy remote BlockQueryServiceClient (the gRPC connection to the
// committer) can return err == nil together with a *committerpb.Block whose
// Header field is unset -- a perfectly valid, zero-value protobuf message.
// GetBlockNumberByTxID must now return a wrapped error instead of panicking
// with a nil-pointer dereference.
func TestGetBlockNumberByTxIDNilHeaderReturnsError(t *testing.T) {
	t.Parallel()

	fakeBlockClient := &mock.BlockQueryServiceClient{}
	fakeQueryService := &mock.QueryService{}
	l := ledger.New(fakeBlockClient, fakeQueryService, context.Background())

	// A malicious/compromised committer returns a "successful" response
	// carrying a Block with no Header at all.
	fakeBlockClient.GetBlockByTxIDReturns(&cb.Block{}, nil)

	require.NotPanics(t, func() {
		_, err := l.GetBlockNumberByTxID("victim-tx")
		require.Error(t, err)
		require.Contains(t, err.Error(), "has no header")
	})
}

// TestBlockProcessedTransactionShortTransactionsFilterReturnsError proves the
// fix: a malicious GetBlockByNumber response whose TRANSACTIONS_FILTER byte
// slice is shorter than Data.Data (fewer status bytes than transactions) must
// now cause ProcessedTransaction to return an error instead of panicking with
// an index-out-of-range.
func TestBlockProcessedTransactionShortTransactionsFilterReturnsError(t *testing.T) {
	t.Parallel()

	fakeBlockClient := &mock.BlockQueryServiceClient{}
	fakeQueryService := &mock.QueryService{}
	l := ledger.New(fakeBlockClient, fakeQueryService, context.Background())

	// Two well-formed transaction envelopes in Data.Data, but only one status
	// byte in the TRANSACTIONS_FILTER metadata entry.
	fakeBlockClient.GetBlockByNumberReturns(&cb.Block{
		Header: &cb.BlockHeader{Number: 1},
		Data: &cb.BlockData{Data: [][]byte{
			validMessageEnvelope(t, "tx0"),
			validMessageEnvelope(t, "tx1"),
		}},
		Metadata: &cb.BlockMetadata{
			Metadata: [][]byte{
				{},     // SIGNATURES
				{},     // LAST_CONFIG
				{0x00}, // TRANSACTIONS_FILTER -- only covers tx index 0
			},
		},
	}, nil)

	block, err := l.GetBlockByNumber(1)
	require.NoError(t, err)

	require.NotPanics(t, func() {
		pt, err := block.ProcessedTransaction(1)
		require.Error(t, err)
		require.Nil(t, pt)
		require.Contains(t, err.Error(), "out of range")
	})
}

// TestBlockDataAtOutOfRangeReturnsNil proves the fix: DataAt has no error
// return in the driver.Block interface, so an out-of-bounds index must return
// nil rather than panicking with an index-out-of-range.
func TestBlockDataAtOutOfRangeReturnsNil(t *testing.T) {
	t.Parallel()

	block := &ledger.Block{
		Block: &cb.Block{
			Data: &cb.BlockData{Data: [][]byte{[]byte("tx0")}},
		},
	}

	require.NotPanics(t, func() {
		require.Nil(t, block.DataAt(1))
		require.Nil(t, block.DataAt(-1))
	})
}

// TestBlockProcessedTransactionMissingMetadataReturnsError proves that a
// block with no Metadata at all (another perfectly valid zero-value protobuf
// shape a compromised committer could return) is rejected with an error
// rather than panicking on a nil Metadata dereference.
func TestBlockProcessedTransactionMissingMetadataReturnsError(t *testing.T) {
	t.Parallel()

	block := &ledger.Block{
		Block: &cb.Block{
			Data: &cb.BlockData{Data: [][]byte{validMessageEnvelope(t, "tx0")}},
		},
	}

	require.NotPanics(t, func() {
		pt, err := block.ProcessedTransaction(0)
		require.Error(t, err)
		require.Nil(t, pt)
		require.Contains(t, err.Error(), "transactions filter")
	})
}
