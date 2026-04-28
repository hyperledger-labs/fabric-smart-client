/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	storage "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

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
			mockCM := &mockChaincodeManager{}
			mockLM := &mockLocalMembership{}
			mockCS := &mockConfigService{}
			mockTM := &mockTransactionManager{}
			l := New("test-channel", mockCM, mockLM, mockCS, mockTM)

			mockInvocation := &mockChaincodeInvocation{}
			mockInvocation.On("Query").Return(tt.queryRes, tt.queryErr).Once()

			mockCC := &mockChaincode{}
			mockCC.On("NewInvocation", mock.Anything, mock.Anything).Return(mockInvocation)

			mockCM.On("Chaincode", mock.Anything).Return(mockCC)
			mockLM.On("DefaultIdentity").Return(view.Identity("alice"))
			mockCS.On("PickPeer", mock.Anything).Return(nil)

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

	txID := "tx1"
	raw := []byte("processed-tx")

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
			queryErr: errors.New("query-failed"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockCM := &mockChaincodeManager{}
			mockLM := &mockLocalMembership{}
			mockCS := &mockConfigService{}
			mockTM := &mockTransactionManager{}
			l := New("test-channel", mockCM, mockLM, mockCS, mockTM)

			mockInvocation := &mockChaincodeInvocation{}
			mockInvocation.On("Query").Return(tt.queryRes, tt.queryErr).Once()

			mockCC := &mockChaincode{}
			mockCC.On("NewInvocation", mock.Anything, mock.Anything, mock.Anything).Return(mockInvocation)

			mockCM.On("Chaincode", mock.Anything).Return(mockCC)
			mockLM.On("DefaultIdentity").Return(view.Identity("alice"))
			mockCS.On("PickPeer", mock.Anything).Return(nil)
			if !tt.wantErr {
				mockTM.On("NewProcessedTransaction", tt.queryRes).Return(&mockProcessedTransaction{}, nil)
			}

			pt, err := l.GetTransactionByID(txID)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, pt)
			}
		})
	}
}

func TestGetBlockNumberByTxID(t *testing.T) {
	t.Parallel()

	txID := "tx1"
	block := &common.Block{
		Header: &common.BlockHeader{
			Number: 5,
		},
	}
	raw, _ := proto.Marshal(block)

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
			name:     "QueryFailure",
			queryRes: nil,
			queryErr: errors.New("query-failed"),
			wantErr:  true,
		},
		{
			name:          "UnmarshalFailure",
			queryRes:      []byte("invalid-proto"),
			queryErr:      nil,
			wantErr:       true,
			expectedError: "unmarshal failed",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockCM := &mockChaincodeManager{}
			mockLM := &mockLocalMembership{}
			mockCS := &mockConfigService{}
			mockTM := &mockTransactionManager{}
			l := New("test-channel", mockCM, mockLM, mockCS, mockTM)

			mockInvocation := &mockChaincodeInvocation{}
			mockInvocation.On("Query").Return(tt.queryRes, tt.queryErr).Once()

			mockCC := &mockChaincode{}
			mockCC.On("NewInvocation", mock.Anything, mock.Anything, mock.Anything).Return(mockInvocation)

			mockCM.On("Chaincode", mock.Anything).Return(mockCC)
			mockLM.On("DefaultIdentity").Return(view.Identity("alice"))
			mockCS.On("PickPeer", mock.Anything).Return(nil)

			num, err := l.GetBlockNumberByTxID(txID)
			if tt.wantErr {
				require.Error(t, err)
				if tt.expectedError != "" {
					require.Contains(t, err.Error(), tt.expectedError)
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, uint64(5), num)
			}
		})
	}
}

func TestGetBlockByNumber(t *testing.T) {
	t.Parallel()

	num := uint64(5)
	block := &common.Block{
		Header: &common.BlockHeader{
			Number: 5,
		},
	}
	raw, _ := proto.Marshal(block)

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
			name:     "QueryFailure",
			queryRes: nil,
			queryErr: errors.New("query-failed"),
			wantErr:  true,
		},
		{
			name:          "UnmarshalFailure",
			queryRes:      []byte("invalid-proto"),
			queryErr:      nil,
			wantErr:       true,
			expectedError: "unmarshal failed",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockCM := &mockChaincodeManager{}
			mockLM := &mockLocalMembership{}
			mockCS := &mockConfigService{}
			mockTM := &mockTransactionManager{}
			l := New("test-channel", mockCM, mockLM, mockCS, mockTM)

			mockInvocation := &mockChaincodeInvocation{}
			mockInvocation.On("Query").Return(tt.queryRes, tt.queryErr).Once()

			mockCC := &mockChaincode{}
			mockCC.On("NewInvocation", mock.Anything, mock.Anything, mock.Anything).Return(mockInvocation)

			mockCM.On("Chaincode", mock.Anything).Return(mockCC)
			mockLM.On("DefaultIdentity").Return(view.Identity("alice"))
			mockCS.On("PickPeer", mock.Anything).Return(nil)

			b, err := l.GetBlockByNumber(num)
			if tt.wantErr {
				require.Error(t, err)
				if tt.expectedError != "" {
					require.Contains(t, err.Error(), tt.expectedError)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, b)
			}
		})
	}
}

func TestBlock(t *testing.T) {
	t.Parallel()
	mockTM := &mockTransactionManager{}
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

	t.Run("DataAt", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, rawEnv, b.DataAt(0))
	})

	t.Run("ProcessedTransaction", func(t *testing.T) {
		t.Parallel()
		mockTM.On("NewProcessedTransaction", mock.Anything).Return(&mockProcessedTransaction{}, nil)
		pt, err := b.ProcessedTransaction(0)
		require.NoError(t, err)
		require.NotNil(t, pt)

		// Error path: unmarshal fails
		block.Data.Data[0] = []byte("invalid-proto")
		_, err = b.ProcessedTransaction(0)
		require.Error(t, err)

		// Error path: NewProcessedTransaction fails
		block.Data.Data[0] = rawEnv
		mockTM.ExpectedCalls = nil // Reset expectations
		mockTM.On("NewProcessedTransaction", mock.Anything).Return(nil, errors.New("failed")).Once()
		_, err = b.ProcessedTransaction(0)
		require.Error(t, err)
	})
}

// Mocks

type mockChaincodeManager struct {
	mock.Mock
}

func (m *mockChaincodeManager) Chaincode(name string) driver.Chaincode {
	args := m.Called(name)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(driver.Chaincode)
}

type mockChaincode struct {
	mock.Mock
}

func (m *mockChaincode) NewInvocation(function string, args ...interface{}) driver.ChaincodeInvocation {
	mockArgs := append([]interface{}{function}, args...)
	callArgs := m.Called(mockArgs...)
	if callArgs.Get(0) == nil {
		return nil
	}
	return callArgs.Get(0).(driver.ChaincodeInvocation)
}

func (m *mockChaincode) NewDiscover() driver.ChaincodeDiscover { return nil }
func (m *mockChaincode) IsAvailable() (bool, error)            { return true, nil }
func (m *mockChaincode) IsPrivate() bool                       { return false }
func (m *mockChaincode) Version() (string, error)              { return "v1", nil }

type mockChaincodeInvocation struct {
	mock.Mock
}

func (m *mockChaincodeInvocation) Endorse() (driver.Envelope, error) { return nil, nil }
func (m *mockChaincodeInvocation) Query() ([]byte, error) {
	args := m.Called()
	res := args.Get(0)
	if res == nil {
		return nil, args.Error(1)
	}
	return res.([]byte), args.Error(1)
}
func (m *mockChaincodeInvocation) Submit() (string, []byte, error) { return "", nil, nil }
func (m *mockChaincodeInvocation) WithTransientEntry(k string, v interface{}) (driver.ChaincodeInvocation, error) {
	return m, nil
}

func (m *mockChaincodeInvocation) WithEndorsersByMSPIDs(mspIDs ...string) driver.ChaincodeInvocation {
	return m
}
func (m *mockChaincodeInvocation) WithEndorsersFromMyOrg() driver.ChaincodeInvocation { return m }
func (m *mockChaincodeInvocation) WithSignerIdentity(id view.Identity) driver.ChaincodeInvocation {
	return m
}

func (m *mockChaincodeInvocation) WithTxID(id driver.TxIDComponents) driver.ChaincodeInvocation {
	return m
}

func (m *mockChaincodeInvocation) WithEndorsersByConnConfig(ccs ...*grpc.ConnectionConfig) driver.ChaincodeInvocation {
	return m
}

func (m *mockChaincodeInvocation) WithImplicitCollections(mspIDs ...string) driver.ChaincodeInvocation {
	return m
}

func (m *mockChaincodeInvocation) WithDiscoveredEndorsersByEndpoints(endpoints ...string) driver.ChaincodeInvocation {
	return m
}
func (m *mockChaincodeInvocation) WithMatchEndorsementPolicy() driver.ChaincodeInvocation { return m }
func (m *mockChaincodeInvocation) WithNumRetries(numRetries uint) driver.ChaincodeInvocation {
	return m
}

func (m *mockChaincodeInvocation) WithRetrySleep(duration time.Duration) driver.ChaincodeInvocation {
	return m
}

func (m *mockChaincodeInvocation) WithContext(ctx context.Context) driver.ChaincodeInvocation {
	return m
}

type mockLocalMembership struct {
	mock.Mock
}

func (m *mockLocalMembership) DefaultIdentity() view.Identity {
	args := m.Called()
	return args.Get(0).(view.Identity)
}
func (m *mockLocalMembership) AnonymousIdentity() (view.Identity, error) { return nil, nil }
func (m *mockLocalMembership) GetIdentityByID(id string) (view.Identity, error) {
	return nil, nil
}
func (m *mockLocalMembership) IsMe(ctx context.Context, id view.Identity) bool { return true }
func (m *mockLocalMembership) DefaultSigningIdentity() driver.SigningIdentity  { return nil }
func (m *mockLocalMembership) RegisterX509MSP(id, path, mspID string) error    { return nil }
func (m *mockLocalMembership) RegisterIdemixMSP(id, path, mspID string) error  { return nil }
func (m *mockLocalMembership) GetIdentityInfoByLabel(mspType, label string) *driver.IdentityInfo {
	return nil
}

func (m *mockLocalMembership) GetIdentityInfoByIdentity(mspType string, id view.Identity) *driver.IdentityInfo {
	return nil
}
func (m *mockLocalMembership) Refresh() error { return nil }

type mockConfigService struct {
	mock.Mock
}

func (m *mockConfigService) PickPeer(role driver.PeerFunctionType) *grpc.ConnectionConfig {
	args := m.Called(role)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*grpc.ConnectionConfig)
}
func (m *mockConfigService) IsChannelConfigReady() bool                               { return true }
func (m *mockConfigService) GetString(key string) string                              { return "" }
func (m *mockConfigService) GetInt(key string) int                                    { return 0 }
func (m *mockConfigService) GetDuration(key string) time.Duration                     { return 0 }
func (m *mockConfigService) GetBool(key string) bool                                  { return false }
func (m *mockConfigService) GetStringSlice(key string) []string                       { return nil }
func (m *mockConfigService) IsSet(key string) bool                                    { return false }
func (m *mockConfigService) UnmarshalKey(key string, rawVal interface{}) error        { return nil }
func (m *mockConfigService) ConfigFileUsed() string                                   { return "" }
func (m *mockConfigService) GetPath(key string) string                                { return "" }
func (m *mockConfigService) TranslatePath(path string) string                         { return "" }
func (m *mockConfigService) NetworkName() string                                      { return "" }
func (m *mockConfigService) DriverName() string                                       { return "" }
func (m *mockConfigService) DefaultChannel() string                                   { return "" }
func (m *mockConfigService) Channel(name string) driver.ChannelConfig                 { return nil }
func (m *mockConfigService) ChannelIDs() []string                                     { return nil }
func (m *mockConfigService) Orderers() []*grpc.ConnectionConfig                       { return nil }
func (m *mockConfigService) OrderingTLSEnabled() (bool, bool)                         { return false, false }
func (m *mockConfigService) OrderingTLSClientAuthRequired() (bool, bool)              { return false, false }
func (m *mockConfigService) SetConfigOrderers([]*grpc.ConnectionConfig) error         { return nil }
func (m *mockConfigService) PickOrderer() *grpc.ConnectionConfig                      { return nil }
func (m *mockConfigService) BroadcastNumRetries() int                                 { return 0 }
func (m *mockConfigService) BroadcastRetryInterval() time.Duration                    { return 0 }
func (m *mockConfigService) OrdererConnectionPoolSize() int                           { return 0 }
func (m *mockConfigService) IsChannelQuiet(name string) bool                          { return false }
func (m *mockConfigService) VaultPersistenceName() storage.PersistenceName            { return "" }
func (m *mockConfigService) VaultTXStoreCacheSize() int                               { return 0 }
func (m *mockConfigService) TLSServerHostOverride() string                            { return "" }
func (m *mockConfigService) ClientConnTimeout() time.Duration                         { return 0 }
func (m *mockConfigService) TLSClientAuthRequired() bool                              { return false }
func (m *mockConfigService) TLSClientKeyFile() string                                 { return "" }
func (m *mockConfigService) TLSClientCertFile() string                                { return "" }
func (m *mockConfigService) ClientKeepAliveConfig() *grpc.ClientKeepAliveConfig       { return nil }
func (m *mockConfigService) NewDefaultChannelConfig(name string) driver.ChannelConfig { return nil }
func (m *mockConfigService) TLSEnabled() bool                                         { return false }

type mockTransactionManager struct {
	mock.Mock
}

func (m *mockTransactionManager) NewProcessedTransaction(raw []byte) (driver.ProcessedTransaction, error) {
	args := m.Called(raw)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(driver.ProcessedTransaction), args.Error(1)
}
func (m *mockTransactionManager) ComputeTxID(id *driver.TxIDComponents) string { return "" }
func (m *mockTransactionManager) NewEnvelope() driver.Envelope                 { return nil }
func (m *mockTransactionManager) NewProposalResponseFromBytes(raw []byte) (driver.ProposalResponse, error) {
	return nil, nil
}

func (m *mockTransactionManager) NewTransaction(ctx context.Context, transactionType driver.TransactionType, creator view.Identity, nonce []byte, txid, channel string, rawRequest []byte) (driver.Transaction, error) {
	return nil, nil
}

func (m *mockTransactionManager) NewTransactionFromBytes(ctx context.Context, channel string, raw []byte) (driver.Transaction, error) {
	return nil, nil
}

func (m *mockTransactionManager) NewTransactionFromEnvelopeBytes(ctx context.Context, channel string, raw []byte) (driver.Transaction, error) {
	return nil, nil
}

func (m *mockTransactionManager) AddTransactionFactory(tt driver.TransactionType, factory driver.TransactionFactory) {
}

func (m *mockTransactionManager) NewProcessedTransactionFromEnvelopePayload(envelopePayload []byte) (driver.ProcessedTransaction, int32, error) {
	return nil, 0, nil
}

func (m *mockTransactionManager) NewProcessedTransactionFromEnvelopeRaw(envelope []byte) (driver.ProcessedTransaction, error) {
	return nil, nil
}

type mockProcessedTransaction struct {
	mock.Mock
}

func (m *mockProcessedTransaction) TxID() string          { return "" }
func (m *mockProcessedTransaction) Results() []byte       { return nil }
func (m *mockProcessedTransaction) ValidationCode() int32 { return 0 }
func (m *mockProcessedTransaction) IsValid() bool         { return true }
func (m *mockProcessedTransaction) Envelope() []byte      { return nil }
