/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ordering/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// mockTransaction implements the Transaction interface for testing
type mockTransaction struct {
	channel           string
	id                string
	creator           view.Identity
	proposal          driver.Proposal
	proposalResponses []driver.ProposalResponse
	bytes             []byte
	envelope          driver.Envelope
	bytesErr          error
	envelopeErr       error
	responsesErr      error
}

func (m *mockTransaction) Channel() string           { return m.channel }
func (m *mockTransaction) ID() string                { return m.id }
func (m *mockTransaction) Creator() view.Identity    { return m.creator }
func (m *mockTransaction) Proposal() driver.Proposal { return m.proposal }
func (m *mockTransaction) ProposalResponses() ([]driver.ProposalResponse, error) {
	return m.proposalResponses, m.responsesErr
}
func (m *mockTransaction) Bytes() ([]byte, error)             { return m.bytes, m.bytesErr }
func (m *mockTransaction) Envelope() (driver.Envelope, error) { return m.envelope, m.envelopeErr }

// mockEnvelope implements driver.Envelope for testing
type mockEnvelope struct {
	bytes    []byte
	bytesErr error
}

func (m *mockEnvelope) Bytes() ([]byte, error) { return m.bytes, m.bytesErr }
func (m *mockEnvelope) FromBytes([]byte) error { return nil }
func (m *mockEnvelope) Results() []byte        { return nil }
func (m *mockEnvelope) TxID() string           { return "" }
func (m *mockEnvelope) Nonce() []byte          { return nil }
func (m *mockEnvelope) Creator() []byte        { return nil }
func (m *mockEnvelope) String() string         { return "" }

// mockTransactionWithEnvelope implements TransactionWithEnvelope for testing
type mockTransactionWithEnvelope struct {
	envelope *common.Envelope
}

func (m *mockTransactionWithEnvelope) Envelope() *common.Envelope {
	return m.envelope
}

// mockEndorserTransactionService implements driver.EndorserTransactionService for testing
type mockEndorserTransactionService struct {
	driver.EndorserTransactionService
}

// mockSignerService implements driver.SignerService for testing
type mockSignerService struct {
	driver.SignerService
}

// mockBroadcaster is a mock broadcaster function
type mockBroadcaster struct {
	mu        sync.Mutex
	called    bool
	callCount int
	err       error
	env       *common.Envelope
}

func (m *mockBroadcaster) broadcast(ctx context.Context, env *common.Envelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called = true
	m.callCount++
	m.env = env
	return m.err
}

// TestNewService tests the Service constructor
func TestNewService(t *testing.T) {
	t.Parallel()

	getEndorserTxService := func(channelID string) (driver.EndorserTransactionService, error) {
		return &mockEndorserTransactionService{}, nil
	}
	sigService := &mockSignerService{}
	configService := &fake.ConfigService{
		PoolSizeValue:    4,
		RetriesValue:     3,
		NetworkNameValue: "test-network",
	}
	m := &metrics.Metrics{}
	services := fakeServices{}

	service := NewService(getEndorserTxService, sigService, configService, m, services)

	require.NotNil(t, service)
	require.NotNil(t, service.GetEndorserTransactionService)
	require.Equal(t, sigService, service.SigService)
	require.Equal(t, configService, service.ConfigService)
	require.Equal(t, m, service.Metrics)
	require.NotNil(t, service.Broadcasters)
	require.Len(t, service.Broadcasters, 3) // BFT, Raft, Solo
	require.NotNil(t, service.Broadcasters[BFT])
	require.NotNil(t, service.Broadcasters[Raft])
	require.NotNil(t, service.Broadcasters[Solo])
	require.Nil(t, service.Broadcaster) // Not set until SetConsensusType is called
}

// TestService_SetConsensusType tests setting the consensus type
func TestService_SetConsensusType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		consensusType ConsensusType
		expectError   bool
	}{
		{
			name:          "set BFT consensus",
			consensusType: BFT,
			expectError:   false,
		},
		{
			name:          "set Raft consensus",
			consensusType: Raft,
			expectError:   false,
		},
		{
			name:          "set Solo consensus",
			consensusType: Solo,
			expectError:   false,
		},
		{
			name:          "set invalid consensus type",
			consensusType: "invalid",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			service := NewService(
				func(channelID string) (driver.EndorserTransactionService, error) {
					return &mockEndorserTransactionService{}, nil
				},
				&mockSignerService{},
				&fake.ConfigService{PoolSizeValue: 4, RetriesValue: 3, NetworkNameValue: "test-network"},
				&metrics.Metrics{},
				fakeServices{},
			)

			err := service.SetConsensusType(tt.consensusType)

			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), "no broadcaster found for consensus")
			} else {
				require.NoError(t, err)
				require.NotNil(t, service.Broadcaster)
			}
		})
	}
}

// TestService_Configure tests the Configure method
func TestService_Configure(t *testing.T) {
	t.Parallel()

	t.Run("successful configuration", func(t *testing.T) {
		t.Parallel()
		configService := &fake.ConfigService{PoolSizeValue: 4, RetriesValue: 3, NetworkNameValue: "test-network"}
		service := NewService(
			func(channelID string) (driver.EndorserTransactionService, error) {
				return &mockEndorserTransactionService{}, nil
			},
			&mockSignerService{},
			configService,
			&metrics.Metrics{},
			fakeServices{},
		)

		orderers := []*grpc.ConnectionConfig{
			{Address: "orderer1:7050"},
			{Address: "orderer2:7050"},
		}

		err := service.Configure(Raft, orderers)
		require.NoError(t, err)
		require.NotNil(t, service.Broadcaster)
		require.Equal(t, orderers, configService.Orderers())
	})

	t.Run("invalid consensus type", func(t *testing.T) {
		t.Parallel()
		service := NewService(
			func(channelID string) (driver.EndorserTransactionService, error) {
				return &mockEndorserTransactionService{}, nil
			},
			&mockSignerService{},
			&fake.ConfigService{PoolSizeValue: 4, RetriesValue: 3, NetworkNameValue: "test-network"},
			&metrics.Metrics{},
			fakeServices{},
		)

		orderers := []*grpc.ConnectionConfig{
			{Address: "orderer1:7050"},
		}

		err := service.Configure("invalid", orderers)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to set consensus type")
	})
}

// TestService_Broadcast tests the Broadcast method
func TestService_Broadcast(t *testing.T) {
	t.Parallel()

	t.Run("broadcast with Transaction type", func(t *testing.T) {
		t.Parallel()
		mockBroadcaster := &mockBroadcaster{}
		service := NewService(
			func(channelID string) (driver.EndorserTransactionService, error) {
				return &mockEndorserTransactionService{}, nil
			},
			&mockSignerService{},
			&fake.ConfigService{PoolSizeValue: 4, RetriesValue: 3, NetworkNameValue: "test-network"},
			&metrics.Metrics{},
			fakeServices{},
		)
		service.Broadcaster = mockBroadcaster.broadcast

		// Create a valid envelope bytes
		validEnvelope := &common.Envelope{
			Payload:   []byte("payload"),
			Signature: []byte("signature"),
		}
		envelopeBytes, err := proto.Marshal(validEnvelope)
		require.NoError(t, err)

		tx := &mockTransaction{
			id:      "tx1",
			channel: "channel1",
			envelope: &mockEnvelope{
				bytes: envelopeBytes,
			},
		}

		err = service.Broadcast(t.Context(), tx)
		require.NoError(t, err)
		require.True(t, mockBroadcaster.called)
		require.NotNil(t, mockBroadcaster.env)
	})

	t.Run("broadcast with TransactionWithEnvelope type", func(t *testing.T) {
		t.Parallel()
		mockBroadcaster := &mockBroadcaster{}
		service := NewService(
			func(channelID string) (driver.EndorserTransactionService, error) {
				return &mockEndorserTransactionService{}, nil
			},
			&mockSignerService{},
			&fake.ConfigService{PoolSizeValue: 4, RetriesValue: 3, NetworkNameValue: "test-network"},
			&metrics.Metrics{},
			fakeServices{},
		)
		service.Broadcaster = mockBroadcaster.broadcast

		env := &common.Envelope{
			Payload:   []byte("payload"),
			Signature: []byte("signature"),
		}
		tx := &mockTransactionWithEnvelope{envelope: env}

		err := service.Broadcast(t.Context(), tx)
		require.NoError(t, err)
		require.True(t, mockBroadcaster.called)
		require.Equal(t, env, mockBroadcaster.env)
	})

	t.Run("broadcast with common.Envelope type", func(t *testing.T) {
		t.Parallel()
		mockBroadcaster := &mockBroadcaster{}
		service := NewService(
			func(channelID string) (driver.EndorserTransactionService, error) {
				return &mockEndorserTransactionService{}, nil
			},
			&mockSignerService{},
			&fake.ConfigService{PoolSizeValue: 4, RetriesValue: 3, NetworkNameValue: "test-network"},
			&metrics.Metrics{},
			fakeServices{},
		)
		service.Broadcaster = mockBroadcaster.broadcast

		env := &common.Envelope{
			Payload:   []byte("payload"),
			Signature: []byte("signature"),
		}

		err := service.Broadcast(t.Context(), env)
		require.NoError(t, err)
		require.True(t, mockBroadcaster.called)
		require.Equal(t, env, mockBroadcaster.env)
	})

	t.Run("broadcast with TODO context", func(t *testing.T) {
		t.Parallel()
		mockBroadcaster := &mockBroadcaster{}
		service := NewService(
			func(channelID string) (driver.EndorserTransactionService, error) {
				return &mockEndorserTransactionService{}, nil
			},
			&mockSignerService{},
			&fake.ConfigService{PoolSizeValue: 4, RetriesValue: 3, NetworkNameValue: "test-network"},
			&metrics.Metrics{},
			fakeServices{},
		)
		service.Broadcaster = mockBroadcaster.broadcast

		env := &common.Envelope{
			Payload:   []byte("payload"),
			Signature: []byte("signature"),
		}

		err := service.Broadcast(context.TODO(), env)
		require.NoError(t, err)
		require.True(t, mockBroadcaster.called)
	})

	t.Run("broadcast with invalid blob type", func(t *testing.T) {
		t.Parallel()
		service := NewService(
			func(channelID string) (driver.EndorserTransactionService, error) {
				return &mockEndorserTransactionService{}, nil
			},
			&mockSignerService{},
			&fake.ConfigService{PoolSizeValue: 4, RetriesValue: 3, NetworkNameValue: "test-network"},
			&metrics.Metrics{},
			fakeServices{},
		)
		service.Broadcaster = func(ctx context.Context, env *common.Envelope) error {
			return nil
		}

		err := service.Broadcast(t.Context(), "invalid type")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid blob's type")
	})

	t.Run("broadcast without broadcaster set", func(t *testing.T) {
		t.Parallel()
		service := NewService(
			func(channelID string) (driver.EndorserTransactionService, error) {
				return &mockEndorserTransactionService{}, nil
			},
			&mockSignerService{},
			&fake.ConfigService{PoolSizeValue: 4, RetriesValue: 3, NetworkNameValue: "test-network"},
			&metrics.Metrics{},
			fakeServices{},
		)
		// Don't set broadcaster

		env := &common.Envelope{
			Payload:   []byte("payload"),
			Signature: []byte("signature"),
		}

		err := service.Broadcast(t.Context(), env)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot broadcast yet, no consensus type set")
	})

	t.Run("broadcast with transaction envelope error", func(t *testing.T) {
		t.Parallel()
		service := NewService(
			func(channelID string) (driver.EndorserTransactionService, error) {
				return &mockEndorserTransactionService{}, nil
			},
			&mockSignerService{},
			&fake.ConfigService{PoolSizeValue: 4, RetriesValue: 3, NetworkNameValue: "test-network"},
			&metrics.Metrics{},
			fakeServices{},
		)
		service.Broadcaster = func(ctx context.Context, env *common.Envelope) error {
			return nil
		}

		tx := &mockTransaction{
			id:          "tx1",
			envelopeErr: errors.New("envelope error"),
		}

		err := service.Broadcast(t.Context(), tx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed creating envelope")
	})

	t.Run("broadcast with transaction bytes error", func(t *testing.T) {
		t.Parallel()
		service := NewService(
			func(channelID string) (driver.EndorserTransactionService, error) {
				return &mockEndorserTransactionService{}, nil
			},
			&mockSignerService{},
			&fake.ConfigService{PoolSizeValue: 4, RetriesValue: 3, NetworkNameValue: "test-network"},
			&metrics.Metrics{},
			fakeServices{},
		)
		service.Broadcaster = func(ctx context.Context, env *common.Envelope) error {
			return nil
		}

		tx := &mockTransaction{
			id: "tx1",
			envelope: &mockEnvelope{
				bytesErr: errors.New("bytes error"),
			},
		}

		err := service.Broadcast(t.Context(), tx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed marshalling envelope")
	})

	t.Run("broadcast with invalid envelope bytes", func(t *testing.T) {
		t.Parallel()
		service := NewService(
			func(channelID string) (driver.EndorserTransactionService, error) {
				return &mockEndorserTransactionService{}, nil
			},
			&mockSignerService{},
			&fake.ConfigService{PoolSizeValue: 4, RetriesValue: 3, NetworkNameValue: "test-network"},
			&metrics.Metrics{},
			fakeServices{},
		)
		service.Broadcaster = func(ctx context.Context, env *common.Envelope) error {
			return nil
		}

		tx := &mockTransaction{
			id: "tx1",
			envelope: &mockEnvelope{
				bytes: []byte("invalid proto bytes"),
			},
		}

		err := service.Broadcast(t.Context(), tx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed unmarshalling envelope")
	})

	t.Run("broadcaster returns error", func(t *testing.T) {
		t.Parallel()
		expectedErr := errors.New("broadcast failed")
		mockBroadcaster := &mockBroadcaster{err: expectedErr}
		service := NewService(
			func(channelID string) (driver.EndorserTransactionService, error) {
				return &mockEndorserTransactionService{}, nil
			},
			&mockSignerService{},
			&fake.ConfigService{PoolSizeValue: 4, RetriesValue: 3, NetworkNameValue: "test-network"},
			&metrics.Metrics{},
			fakeServices{},
		)
		service.Broadcaster = mockBroadcaster.broadcast

		env := &common.Envelope{
			Payload:   []byte("payload"),
			Signature: []byte("signature"),
		}

		err := service.Broadcast(t.Context(), env)
		require.Error(t, err)
		require.Equal(t, expectedErr, err)
		require.True(t, mockBroadcaster.called)
	})
}

// TestService_ConcurrentBroadcast tests concurrent broadcast operations
func TestService_ConcurrentBroadcast(t *testing.T) {
	t.Parallel()

	mockBroadcaster := &mockBroadcaster{}
	service := NewService(
		func(channelID string) (driver.EndorserTransactionService, error) {
			return &mockEndorserTransactionService{}, nil
		},
		&mockSignerService{},
		&fake.ConfigService{PoolSizeValue: 4, RetriesValue: 3, NetworkNameValue: "test-network"},
		&metrics.Metrics{},
		fakeServices{},
	)
	service.Broadcaster = mockBroadcaster.broadcast

	const numGoroutines = 10
	errChan := make(chan error, numGoroutines)

	for range numGoroutines {
		go func() {
			env := &common.Envelope{
				Payload:   []byte("payload"),
				Signature: []byte("signature"),
			}
			err := service.Broadcast(t.Context(), env)
			errChan <- err
		}()
	}

	for range numGoroutines {
		err := <-errChan
		require.NoError(t, err)
	}

	require.Equal(t, numGoroutines, mockBroadcaster.callCount)
}

// TestService_ConcurrentSetConsensusType tests concurrent consensus type changes
func TestService_ConcurrentSetConsensusType(t *testing.T) {
	t.Parallel()

	service := NewService(
		func(channelID string) (driver.EndorserTransactionService, error) {
			return &mockEndorserTransactionService{}, nil
		},
		&mockSignerService{},
		&fake.ConfigService{PoolSizeValue: 4, RetriesValue: 3, NetworkNameValue: "test-network"},
		&metrics.Metrics{},
		fakeServices{},
	)

	const numGoroutines = 10
	errChan := make(chan error, numGoroutines)

	consensusTypes := []ConsensusType{BFT, Raft, Solo}

	for i := range numGoroutines {
		go func(i int) {
			consensusType := consensusTypes[i%len(consensusTypes)]
			err := service.SetConsensusType(consensusType)
			errChan <- err
		}(i)
	}

	for range numGoroutines {
		err := <-errChan
		require.NoError(t, err)
	}

	// Verify that a broadcaster is set
	require.NotNil(t, service.Broadcaster)
}

// TestConsensusTypeConstants tests the consensus type constants
func TestConsensusTypeConstants(t *testing.T) {
	t.Parallel()

	require.Equal(t, "BFT", BFT)
	require.Equal(t, "etcdraft", Raft)
	require.Equal(t, "solo", Solo)
}
