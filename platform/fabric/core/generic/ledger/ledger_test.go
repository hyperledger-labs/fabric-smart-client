/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

//go:generate counterfeiter -o mock/chaincode.go -fake-name Chaincode github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Chaincode
//go:generate counterfeiter -o mock/chaincode_invocation.go -fake-name ChaincodeInvocation github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.ChaincodeInvocation
//go:generate counterfeiter -o mock/chaincode_manager.go -fake-name ChaincodeManager github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.ChaincodeManager
//go:generate counterfeiter -o mock/local_membership.go -fake-name LocalMembership github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.LocalMembership
//go:generate counterfeiter -o mock/config_service.go -fake-name ConfigService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.ConfigService
//go:generate counterfeiter -o mock/processed_transaction.go -fake-name ProcessedTransaction github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.ProcessedTransaction
//go:generate counterfeiter -o mock/transaction_manager.go -fake-name TransactionManager github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.TransactionManager

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ledger/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func setupTestLedger(t *testing.T) (*Ledger, *mock.ChaincodeManager, *mock.LocalMembership, *mock.ConfigService, *mock.TransactionManager) {
	t.Helper()
	mockCM := &mock.ChaincodeManager{}
	mockLM := &mock.LocalMembership{}
	mockCS := &mock.ConfigService{}
	mockTM := &mock.TransactionManager{}
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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l, mockCM, mockLM, mockCS, _ := setupTestLedger(t)

			mockInvocation := &mock.ChaincodeInvocation{}
			mockInvocation.QueryReturns(tt.queryRes, tt.queryErr)
			mockInvocation.WithSignerIdentityReturns(mockInvocation)
			mockInvocation.WithEndorsersByConnConfigReturns(mockInvocation)

			mockCC := &mock.Chaincode{}
			mockCC.NewInvocationReturns(mockInvocation)

			mockCM.ChaincodeReturns(mockCC)
			mockLM.DefaultIdentityReturns(view.Identity("alice"))
			mockCS.PickPeerReturns(&grpc.ConnectionConfig{Address: "peer0:7051"})

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
			queryRes: mustMarshal(t, &peer.ProcessedTransaction{ValidationCode: int32(peer.TxValidationCode_VALID)}),
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
			queryRes: []byte("not a processed transaction"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l, mockCM, mockLM, mockCS, mockTM := setupTestLedger(t)

			mockInvocation := &mock.ChaincodeInvocation{}
			mockInvocation.QueryReturns(tt.queryRes, tt.queryErr)
			mockInvocation.WithSignerIdentityReturns(mockInvocation)
			mockInvocation.WithEndorsersByConnConfigReturns(mockInvocation)

			mockCC := &mock.Chaincode{}
			mockCC.NewInvocationReturns(mockInvocation)

			mockCM.ChaincodeReturns(mockCC)
			mockLM.DefaultIdentityReturns(view.Identity("alice"))
			mockCS.PickPeerReturns(&grpc.ConnectionConfig{Address: "peer0:7051"})

			mockPT := &mock.ProcessedTransaction{}
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

// TestGetTransactionByIDShadowed checks that a lookup whose first envelope is
// invalid returns the valid endorser transaction with the same txID in its
// block, as committer.ShadowedEnvelopes rules, and the first envelope
// otherwise. A valid lookup issues no block query, and a block that cannot be
// fetched or parsed is an error, never the forgery's verdict.
func TestGetTransactionByIDShadowed(t *testing.T) {
	t.Parallel()
	const txID = "tx1"
	forged := envelope(t, common.HeaderType_ENDORSER_TRANSACTION, txID, "forged")
	forgedMessage := envelope(t, common.HeaderType_MESSAGE, txID, "forged")
	honest := envelope(t, common.HeaderType_ENDORSER_TRANSACTION, txID, "honest")
	config := envelope(t, common.HeaderType_CONFIG, txID, "config")
	other := envelope(t, common.HeaderType_ENDORSER_TRANSACTION, "tx2", "other")
	garbage := &common.Envelope{Payload: []byte("not a payload")}

	tests := []struct {
		name    string
		lookup  *common.Envelope
		code    peer.TxValidationCode
		block   []byte // GetBlockByTxID result; nil fails the query
		want    *common.Envelope
		wantVC  peer.TxValidationCode
		wantErr string
	}{
		{
			name:   "ValidLookup",
			lookup: honest, code: peer.TxValidationCode_VALID,
			want: honest, wantVC: peer.TxValidationCode_VALID,
		},
		{
			name:   "ForgedEndorserThenHonest",
			lookup: forged, code: peer.TxValidationCode_BAD_PROPOSAL_TXID,
			block: blockOf(t, []*common.Envelope{forged, honest}, peer.TxValidationCode_BAD_PROPOSAL_TXID, peer.TxValidationCode_VALID),
			want:  honest, wantVC: peer.TxValidationCode_VALID,
		},
		{
			name:   "ForgedMessageThenHonest",
			lookup: forgedMessage, code: peer.TxValidationCode_UNSUPPORTED_TX_PAYLOAD,
			block: blockOf(t, []*common.Envelope{forgedMessage, other, honest}, peer.TxValidationCode_UNSUPPORTED_TX_PAYLOAD, peer.TxValidationCode_VALID, peer.TxValidationCode_VALID),
			want:  honest, wantVC: peer.TxValidationCode_VALID,
		},
		{
			name:   "UnparsableEnvelopeBeforeHonest",
			lookup: forged, code: peer.TxValidationCode_BAD_PROPOSAL_TXID,
			block: blockOf(t, []*common.Envelope{forged, garbage, honest}, peer.TxValidationCode_BAD_PROPOSAL_TXID, peer.TxValidationCode_VALID, peer.TxValidationCode_VALID),
			want:  honest, wantVC: peer.TxValidationCode_VALID,
		},
		{
			name:   "NoValidTwin",
			lookup: forged, code: peer.TxValidationCode_MVCC_READ_CONFLICT,
			block: blockOf(t, []*common.Envelope{forged, other}, peer.TxValidationCode_MVCC_READ_CONFLICT, peer.TxValidationCode_VALID),
			want:  forged, wantVC: peer.TxValidationCode_MVCC_READ_CONFLICT,
		},
		{
			// The twin is invalid too, so it does not shadow the forgery and
			// the lookup keeps the first envelope. The block stores the
			// forgery as an unparsable envelope, so only the twin can match.
			name:   "InvalidTwinDoesNotShadow",
			lookup: forged, code: peer.TxValidationCode_BAD_PROPOSAL_TXID,
			block: blockOf(t, []*common.Envelope{garbage, honest}, peer.TxValidationCode_BAD_PROPOSAL_TXID, peer.TxValidationCode_MVCC_READ_CONFLICT),
			want:  forged, wantVC: peer.TxValidationCode_BAD_PROPOSAL_TXID,
		},
		{
			name:   "ValidConfigDoesNotShadow",
			lookup: forged, code: peer.TxValidationCode_BAD_PROPOSAL_TXID,
			block: blockOf(t, []*common.Envelope{forged, config}, peer.TxValidationCode_BAD_PROPOSAL_TXID, peer.TxValidationCode_VALID),
			want:  forged, wantVC: peer.TxValidationCode_BAD_PROPOSAL_TXID,
		},
		{
			name:   "TwinBeyondValidationFlags",
			lookup: forged, code: peer.TxValidationCode_BAD_PROPOSAL_TXID,
			block: blockOf(t, []*common.Envelope{forged, honest}, peer.TxValidationCode_BAD_PROPOSAL_TXID),
			want:  forged, wantVC: peer.TxValidationCode_BAD_PROPOSAL_TXID,
		},
		{
			name:   "NoTransactionsFilter",
			lookup: forged, code: peer.TxValidationCode_BAD_PROPOSAL_TXID,
			block: mustMarshal(t, &common.Block{Data: &common.BlockData{Data: [][]byte{mustMarshal(t, forged), mustMarshal(t, honest)}}}),
			want:  forged, wantVC: peer.TxValidationCode_BAD_PROPOSAL_TXID,
		},
		{
			// Without a valid twin, the parse error of a non-endorser
			// forgery is returned, as before.
			name:   "MessageForgeryWithoutTwin",
			lookup: forgedMessage, code: peer.TxValidationCode_UNSUPPORTED_TX_PAYLOAD,
			block:   blockOf(t, []*common.Envelope{forgedMessage, other}, peer.TxValidationCode_UNSUPPORTED_TX_PAYLOAD, peer.TxValidationCode_VALID),
			wantErr: "only EndorserClient Transactions are supported",
		},
		{
			name:   "BlockQueryFailure",
			lookup: forged, code: peer.TxValidationCode_BAD_PROPOSAL_TXID,
			wantErr: "failed fetching block of transaction [tx1]",
		},
		{
			name:   "BlockUnmarshalFailure",
			lookup: forged, code: peer.TxValidationCode_BAD_PROPOSAL_TXID,
			block:   []byte("not a block"),
			wantErr: "failed unmarshalling block of transaction [tx1]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l, mockCM, mockLM, mockCS, mockTM := setupTestLedger(t)
			mockLM.DefaultIdentityReturns(view.Identity("alice"))
			queryPeer := &grpc.ConnectionConfig{Address: "peer0:7051"}
			mockCS.PickPeerReturns(queryPeer)

			// Each qscc function gets its own invocation.
			lookup := mustMarshal(t, &peer.ProcessedTransaction{TransactionEnvelope: tt.lookup, ValidationCode: int32(tt.code)})
			var blockQueries atomic.Int32
			mockCC := &mock.Chaincode{}
			mockCC.NewInvocationStub = func(function string, _ ...any) driver.ChaincodeInvocation {
				inv := &mock.ChaincodeInvocation{}
				inv.WithEndorsersByConnConfigStub = func(ccs ...*grpc.ConnectionConfig) driver.ChaincodeInvocation {
					require.Equal(t, []*grpc.ConnectionConfig{queryPeer}, ccs)
					return inv
				}
				inv.WithSignerIdentityReturns(inv)
				switch {
				case function == GetTransactionByID:
					inv.QueryReturns(lookup, nil)
				case function == GetBlockByTxID && tt.block == nil:
					blockQueries.Add(1)
					inv.QueryReturns(nil, errors.New("peer unavailable"))
				case function == GetBlockByTxID:
					blockQueries.Add(1)
					inv.QueryReturns(tt.block, nil)
				}
				return inv
			}
			mockCM.ChaincodeReturns(mockCC)
			// The fake reports the envelope and code it was built from, so the
			// test sees which one the ledger chose. Like the transaction
			// manager, it rejects an envelope that is not an endorser
			// transaction.
			mockTM.NewProcessedTransactionStub = func(raw []byte) (driver.ProcessedTransaction, error) {
				pt := &peer.ProcessedTransaction{}
				if err := proto.Unmarshal(raw, pt); err != nil {
					return nil, err
				}
				_, _, chdr, err := fabricutils.UnmarshalTx(mustMarshal(t, pt.GetTransactionEnvelope()))
				if err != nil {
					return nil, err
				}
				if common.HeaderType(chdr.GetType()) != common.HeaderType_ENDORSER_TRANSACTION {
					return nil, errors.New("only EndorserClient Transactions are supported")
				}
				res := &mock.ProcessedTransaction{}
				res.EnvelopeReturns(mustMarshal(t, pt.GetTransactionEnvelope()))
				res.ValidationCodeReturns(pt.GetValidationCode())
				res.IsValidReturns(pt.GetValidationCode() == int32(peer.TxValidationCode_VALID))
				return res, nil
			}

			res, err := l.GetTransactionByID(txID)
			require.Equal(t, 1, mockCS.PickPeerCallCount(), "both queries go to the same peer")
			wantQueries := int32(1)
			if tt.code == peer.TxValidationCode_VALID {
				wantQueries = 0
			}
			require.Equal(t, wantQueries, blockQueries.Load(), "block queries")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, res)
				return
			}
			require.NoError(t, err)
			require.Equal(t, mustMarshal(t, tt.want), res.Envelope())
			require.Equal(t, int32(tt.wantVC), res.ValidationCode())
		})
	}
}

// envelope returns an envelope with the given header type and txID whose
// payload data is tag, so that envelopes with the same txID differ.
func envelope(t *testing.T, typ common.HeaderType, txID, tag string) *common.Envelope {
	t.Helper()
	chdr := mustMarshal(t, &common.ChannelHeader{Type: int32(typ), TxId: txID})
	payload := mustMarshal(t, &common.Payload{Header: &common.Header{ChannelHeader: chdr}, Data: []byte(tag)})
	return &common.Envelope{Payload: payload}
}

// blockOf returns the bytes of a block holding envs with the given validation
// codes.
func blockOf(t *testing.T, envs []*common.Envelope, codes ...peer.TxValidationCode) []byte {
	t.Helper()
	data := make([][]byte, len(envs))
	for i, env := range envs {
		data[i] = mustMarshal(t, env)
	}
	flags := make([]byte, len(codes))
	for i, code := range codes {
		flags[i] = byte(code) //nolint:gosec // validation codes fit in a byte, as in the transactions filter
	}
	metadata := make([][]byte, int(common.BlockMetadataIndex_TRANSACTIONS_FILTER)+1)
	metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = flags
	return mustMarshal(t, &common.Block{
		Header:   &common.BlockHeader{Number: 7},
		Data:     &common.BlockData{Data: data},
		Metadata: &common.BlockMetadata{Metadata: metadata},
	})
}

func mustMarshal(t *testing.T, m proto.Message) []byte {
	t.Helper()
	raw, err := proto.Marshal(m)
	require.NoError(t, err)
	return raw
}

func TestGetBlockNumberByTxID(t *testing.T) {
	t.Parallel()

	block := &common.Block{
		Header: &common.BlockHeader{
			Number: 10,
		},
	}
	raw, _ := proto.Marshal(block)

	// A Block with no Header is a valid protobuf encoding, so a peer can
	// return one that unmarshals cleanly.
	noHeaderRaw, err := proto.Marshal(&common.Block{})
	require.NoError(t, err)

	tests := []struct {
		name        string
		queryRes    []byte
		queryErr    error
		wantErr     bool
		errContains string
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
			name:        "NilHeader",
			queryRes:    noHeaderRaw,
			queryErr:    nil,
			wantErr:     true,
			errContains: "has no header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l, mockCM, mockLM, mockCS, _ := setupTestLedger(t)

			mockInvocation := &mock.ChaincodeInvocation{}
			mockInvocation.QueryReturns(tt.queryRes, tt.queryErr)
			mockInvocation.WithSignerIdentityReturns(mockInvocation)
			mockInvocation.WithEndorsersByConnConfigReturns(mockInvocation)

			mockCC := &mock.Chaincode{}
			mockCC.NewInvocationReturns(mockInvocation)

			mockCM.ChaincodeReturns(mockCC)
			mockLM.DefaultIdentityReturns(view.Identity("alice"))
			mockCS.PickPeerReturns(&grpc.ConnectionConfig{Address: "peer0:7051"})

			res, err := l.GetBlockNumberByTxID("tx1")
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.ErrorContains(t, err, tt.errContains)
				}
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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l, mockCM, mockLM, mockCS, _ := setupTestLedger(t)

			mockInvocation := &mock.ChaincodeInvocation{}
			mockInvocation.QueryReturns(tt.queryRes, tt.queryErr)
			mockInvocation.WithSignerIdentityReturns(mockInvocation)
			mockInvocation.WithEndorsersByConnConfigReturns(mockInvocation)

			mockCC := &mock.Chaincode{}
			mockCC.NewInvocationReturns(mockInvocation)

			mockCM.ChaincodeReturns(mockCC)
			mockLM.DefaultIdentityReturns(view.Identity("alice"))
			mockCS.PickPeerReturns(&grpc.ConnectionConfig{Address: "peer0:7051"})

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

	setup := func() (*Block, *mock.TransactionManager, []byte) {
		mockTM := &mock.TransactionManager{}
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
		mockTM.NewProcessedTransactionReturns(&mock.ProcessedTransaction{}, nil)
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

	// Regression test: ProcessedTransaction used to index straight into
	// b.Data.Data[i] and b.Metadata.Metadata[TRANSACTIONS_FILTER][i] with no
	// bounds checking. A malformed/malicious block (e.g. one whose
	// TRANSACTIONS_FILTER metadata entry is shorter than Data.Data, or an
	// out-of-range index requested by a caller) crashes the whole process with
	// an index-out-of-range panic instead of returning an error.
	t.Run("out of range index does not panic", func(t *testing.T) {
		t.Parallel()
		b, _, _ := setup()

		require.NotPanics(t, func() {
			_, err := b.ProcessedTransaction(5)
			require.Error(t, err)
		})
	})

	t.Run("undersized transactions filter does not panic", func(t *testing.T) {
		t.Parallel()
		mockTM := &mock.TransactionManager{}
		env := &common.Envelope{Payload: []byte("payload1")}
		rawEnv, _ := proto.Marshal(env)

		// Two data entries but the TRANSACTIONS_FILTER metadata entry only
		// covers the first one.
		block := &common.Block{
			Data: &common.BlockData{
				Data: [][]byte{rawEnv, rawEnv},
			},
			Metadata: &common.BlockMetadata{
				Metadata: [][]byte{
					nil,
					nil,
					{byte(peer.TxValidationCode_VALID)},
				},
			},
		}
		b := &Block{Block: block, TransactionManager: mockTM}

		require.NotPanics(t, func() {
			_, err := b.ProcessedTransaction(1)
			require.Error(t, err)
		})
	})

	t.Run("missing metadata does not panic", func(t *testing.T) {
		t.Parallel()
		mockTM := &mock.TransactionManager{}
		env := &common.Envelope{Payload: []byte("payload1")}
		rawEnv, _ := proto.Marshal(env)

		block := &common.Block{
			Data: &common.BlockData{
				Data: [][]byte{rawEnv},
			},
			Metadata: &common.BlockMetadata{
				Metadata: [][]byte{},
			},
		}
		b := &Block{Block: block, TransactionManager: mockTM}

		require.NotPanics(t, func() {
			_, err := b.ProcessedTransaction(0)
			require.Error(t, err)
		})
	})

	// Regression test: block.Data and block.Metadata are populated
	// independently by whoever produced the block, so a malformed block can
	// have either be nil entirely - not just shorter than expected.
	t.Run("nil Data does not panic", func(t *testing.T) {
		t.Parallel()
		mockTM := &mock.TransactionManager{}
		block := &common.Block{
			Metadata: &common.BlockMetadata{
				Metadata: [][]byte{nil, nil, {byte(peer.TxValidationCode_VALID)}},
			},
		}
		b := &Block{Block: block, TransactionManager: mockTM}

		require.NotPanics(t, func() {
			require.Nil(t, b.DataAt(0))
			_, err := b.ProcessedTransaction(0)
			require.Error(t, err)
		})
	})

	t.Run("nil Metadata does not panic", func(t *testing.T) {
		t.Parallel()
		mockTM := &mock.TransactionManager{}
		env := &common.Envelope{Payload: []byte("payload1")}
		rawEnv, _ := proto.Marshal(env)

		block := &common.Block{
			Data: &common.BlockData{
				Data: [][]byte{rawEnv},
			},
		}
		b := &Block{Block: block, TransactionManager: mockTM}

		require.NotPanics(t, func() {
			require.Equal(t, rawEnv, b.DataAt(0))
			_, err := b.ProcessedTransaction(0)
			require.Error(t, err)
		})
	})

	t.Run("DataAt out of range returns nil instead of panicking", func(t *testing.T) {
		t.Parallel()
		b, _, _ := setup()

		require.NotPanics(t, func() {
			require.Nil(t, b.DataAt(-1))
			require.Nil(t, b.DataAt(5))
		})
	})
}

// PickPeer returns nil when no peer is configured for queries. The nil used to
// be handed to WithEndorsersByConnConfig, where it survives the
// len(EndorsersByConnConfig) != 0 guard - one nil passed to a variadic is a
// slice of length one - and is dereferenced at chaincode/invoke.go:283. Every
// query entry point must report the misconfiguration instead.
func TestQueryChaincode_noPeerConfiguredForQuery(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		call func(l *Ledger) error
	}{
		{"GetLedgerInfo", func(l *Ledger) error { _, err := l.GetLedgerInfo(); return err }},
		{"GetTransactionByID", func(l *Ledger) error { _, err := l.GetTransactionByID("tx1"); return err }},
		{"GetBlockNumberByTxID", func(l *Ledger) error { _, err := l.GetBlockNumberByTxID("tx1"); return err }},
		{"GetBlockByNumber", func(l *Ledger) error { _, err := l.GetBlockByNumber(1); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l, mockCM, mockLM, mockCS, _ := setupTestLedger(t)

			// A real invocation dereferences each config; the mock would not, so
			// assert on the error rather than merely on the absence of a panic.
			mockInvocation := &mock.ChaincodeInvocation{}
			mockInvocation.WithSignerIdentityReturns(mockInvocation)
			mockInvocation.WithEndorsersByConnConfigReturns(mockInvocation)
			mockInvocation.QueryReturns([]byte("unused"), nil)

			mockCC := &mock.Chaincode{}
			mockCC.NewInvocationReturns(mockInvocation)
			mockCM.ChaincodeReturns(mockCC)
			mockLM.DefaultIdentityReturns(view.Identity("alice"))
			mockCS.PickPeerReturns(nil)

			err := tc.call(l)
			require.ErrorContains(t, err, "no peer configured for query")
			require.Equal(t, 0, mockInvocation.QueryCallCount(),
				"must fail before invoking the chaincode")
		})
	}
}
