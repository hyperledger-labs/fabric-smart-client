/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"errors"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ledger/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func setupTestLedger(t *testing.T) (*Ledger, *mock.FakeChaincodeManager, *mock.FakeLocalMembership, *mock.FakeConfigService, *mock.FakeTransactionManager) {
	t.Helper()
	mockCM := &mock.FakeChaincodeManager{}
	mockLM := &mock.FakeLocalMembership{}
	mockCS := &mock.FakeConfigService{}
	mockTM := &mock.FakeTransactionManager{}
	l := New("test-channel", mockCM, mockLM, mockCS, mockTM)
	return l, mockCM, mockLM, mockCS, mockTM
}

func TestGetLedgerInfo(t *testing.T) {
	t.Parallel()

	bi := &common.BlockchainInfo{
		Height:            10,
		CurrentBlockHash:  []byte("current"),
		PreviousBlockHash: []byte("previous"),
	}
	raw, _ := proto.Marshal(bi)

	tests := []struct {
		name          string
		queryRes      []byte
		queryErr      error
		wantErr       bool
		expectedError string
	}{
		{
			name:     "Success",
			queryRes: raw,
			queryErr: nil,
			wantErr:  false,
		},
		{
			name:          "QueryFailure",
			queryRes:      nil,
			queryErr:      errors.New("query-failed"),
			wantErr:       true,
			expectedError: "failed querying chain info",
		},
		{
			name:          "UnmarshalFailure",
			queryRes:      []byte("invalid-proto"),
			queryErr:      nil,
			wantErr:       true,
			expectedError: "failed unmarshalling block info",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l, mockCM, mockLM, mockCS, _ := setupTestLedger(t)

			mockInvocation := &mock.FakeChaincodeInvocation{}
			mockInvocation.QueryReturns(tt.queryRes, tt.queryErr)
			mockInvocation.WithSignerIdentityReturns(mockInvocation)
			mockInvocation.WithEndorsersByConnConfigReturns(mockInvocation)

			mockCC := &mock.FakeChaincode{}
			mockCC.NewInvocationReturns(mockInvocation)

			mockCM.ChaincodeReturns(mockCC)
			mockLM.DefaultIdentityReturns(view.Identity("alice"))
			mockCS.PickPeerReturns(nil)

			info, err := l.GetLedgerInfo()
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				require.NotNil(t, info)
				require.Equal(t, uint64(10), info.Height)
			}
		})
	}
}

func TestGetTransactionByID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		queryRes []byte
		queryErr error
		wantErr  bool
	}{
		{
			name:     "Success",
			queryRes: []byte("tx-data"),
			queryErr: nil,
			wantErr:  false,
		},
		{
			name:     "QueryFailure",
			queryRes: nil,
			queryErr: errors.New("failed"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l, mockCM, mockLM, mockCS, mockTM := setupTestLedger(t)

			mockInvocation := &mock.FakeChaincodeInvocation{}
			mockInvocation.QueryReturns(tt.queryRes, tt.queryErr)
			mockInvocation.WithSignerIdentityReturns(mockInvocation)
			mockInvocation.WithEndorsersByConnConfigReturns(mockInvocation)

			mockCC := &mock.FakeChaincode{}
			mockCC.NewInvocationReturns(mockInvocation)

			mockCM.ChaincodeReturns(mockCC)
			mockLM.DefaultIdentityReturns(view.Identity("alice"))
			mockCS.PickPeerReturns(nil)

			mockPT := &mock.FakeProcessedTransaction{}
			mockTM.NewProcessedTransactionReturns(mockPT, nil)

			res, err := l.GetTransactionByID("tx1")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, mockPT, res)
			}
		})
	}
}

func TestGetBlockNumberByTxID(t *testing.T) {
	t.Parallel()

	block := &common.Block{
		Header: &common.BlockHeader{
			Number: 10,
		},
	}
	raw, _ := proto.Marshal(block)

	tests := []struct {
		name     string
		queryRes []byte
		queryErr error
		wantErr  bool
	}{
		{
			name:     "Success",
			queryRes: raw,
			queryErr: nil,
			wantErr:  false,
		},
		{
			name:     "QueryFailure",
			queryRes: nil,
			queryErr: errors.New("failed"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l, mockCM, mockLM, mockCS, _ := setupTestLedger(t)

			mockInvocation := &mock.FakeChaincodeInvocation{}
			mockInvocation.QueryReturns(tt.queryRes, tt.queryErr)
			mockInvocation.WithSignerIdentityReturns(mockInvocation)
			mockInvocation.WithEndorsersByConnConfigReturns(mockInvocation)

			mockCC := &mock.FakeChaincode{}
			mockCC.NewInvocationReturns(mockInvocation)

			mockCM.ChaincodeReturns(mockCC)
			mockLM.DefaultIdentityReturns(view.Identity("alice"))
			mockCS.PickPeerReturns(nil)

			res, err := l.GetBlockNumberByTxID("tx1")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, uint64(10), res)
			}
		})
	}
}

func TestGetBlockByNumber(t *testing.T) {
	t.Parallel()

	block := &common.Block{
		Data: &common.BlockData{
			Data: [][]byte{[]byte("data")},
		},
	}
	raw, _ := proto.Marshal(block)

	tests := []struct {
		name     string
		queryRes []byte
		queryErr error
		wantErr  bool
	}{
		{
			name:     "Success",
			queryRes: raw,
			queryErr: nil,
			wantErr:  false,
		},
		{
			name:     "QueryFailure",
			queryRes: nil,
			queryErr: errors.New("failed"),
			wantErr:  true,
		},
		{
			name:     "UnmarshalFailure",
			queryRes: []byte("invalid"),
			queryErr: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l, mockCM, mockLM, mockCS, _ := setupTestLedger(t)

			mockInvocation := &mock.FakeChaincodeInvocation{}
			mockInvocation.QueryReturns(tt.queryRes, tt.queryErr)
			mockInvocation.WithSignerIdentityReturns(mockInvocation)
			mockInvocation.WithEndorsersByConnConfigReturns(mockInvocation)

			mockCC := &mock.FakeChaincode{}
			mockCC.NewInvocationReturns(mockInvocation)

			mockCM.ChaincodeReturns(mockCC)
			mockLM.DefaultIdentityReturns(view.Identity("alice"))
			mockCS.PickPeerReturns(nil)

			res, err := l.GetBlockByNumber(10)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
			}
		})
	}
}

func TestBlock(t *testing.T) {
	t.Parallel()

	setup := func() (*Block, *mock.FakeTransactionManager, []byte) {
		mockTM := &mock.FakeTransactionManager{}
		env := &common.Envelope{Payload: []byte("payload1")}
		rawEnv, _ := proto.Marshal(env)

		block := &common.Block{
			Data: &common.BlockData{
				Data: [][]byte{rawEnv},
			},
			Metadata: &common.BlockMetadata{
				Metadata: [][]byte{
					nil,
					nil,
					{byte(peer.TxValidationCode_VALID)},
				},
			},
		}

		b := &Block{
			Block:              block,
			TransactionManager: mockTM,
		}
		return b, mockTM, rawEnv
	}

	t.Run("DataAt", func(t *testing.T) {
		t.Parallel()
		b, _, rawEnv := setup()
		require.Equal(t, rawEnv, b.DataAt(0))
	})

	t.Run("ProcessedTransaction", func(t *testing.T) {
		t.Parallel()
		b, mockTM, rawEnv := setup()
		mockTM.NewProcessedTransactionReturns(&mock.FakeProcessedTransaction{}, nil)
		pt, err := b.ProcessedTransaction(0)
		require.NoError(t, err)
		require.NotNil(t, pt)

		// Error path: unmarshal fails
		b.Data.Data[0] = []byte("invalid-proto")
		_, err = b.ProcessedTransaction(0)
		require.Error(t, err)

		// Error path: NewProcessedTransaction fails
		b.Data.Data[0] = rawEnv
		mockTM.NewProcessedTransactionReturns(nil, errors.New("failed"))
		_, err = b.ProcessedTransaction(0)
		require.Error(t, err)
	})
}
