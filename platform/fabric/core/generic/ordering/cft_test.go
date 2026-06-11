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
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/stretchr/testify/require"
	ggrpc "google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ordering/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

// fakeCFTServices provides orderer clients for testing
type fakeCFTServices struct {
	shouldFailClient bool
	shouldFailStream bool
	clientError      error
	streamError      error
}

func (f *fakeCFTServices) NewOrdererClient(grpc.ConnectionConfig) (Client, error) {
	if f.shouldFailClient {
		if f.clientError != nil {
			return nil, f.clientError
		}
		return nil, errors.New("client creation failed")
	}
	return &fakeCFTClient{
		shouldFailStream: f.shouldFailStream,
		streamError:      f.streamError,
	}, nil
}

type fakeCFTClient struct {
	Client
	shouldFailStream bool
	streamError      error
	closed           bool
}

func (f *fakeCFTClient) OrdererClient() (ab.AtomicBroadcastClient, error) {
	if f.shouldFailStream {
		if f.streamError != nil {
			return nil, f.streamError
		}
		return nil, errors.New("stream creation failed")
	}
	return &fakeCFTAB{}, nil
}

func (f *fakeCFTClient) Close() {
	f.closed = true
}

type fakeCFTAB struct{}

func (fakeCFTAB) Broadcast(context.Context, ...ggrpc.CallOption) (ggrpc.BidiStreamingClient[common.Envelope, ab.BroadcastResponse], error) {
	return &fakeCFTStream{}, nil
}

func (fakeCFTAB) Deliver(context.Context, ...ggrpc.CallOption) (ggrpc.BidiStreamingClient[common.Envelope, ab.DeliverResponse], error) {
	return nil, nil
}

type fakeCFTStream struct {
	ggrpc.BidiStreamingClient[common.Envelope, ab.BroadcastResponse]
	sendErr  error
	recvErr  error
	status   common.Status
	sendCall int
	recvCall int
	mu       sync.Mutex
}

func (f *fakeCFTStream) Send(*common.Envelope) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sendCall++
	return f.sendErr
}

func (f *fakeCFTStream) Recv() (*ab.BroadcastResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recvCall++
	if f.recvErr != nil {
		return nil, f.recvErr
	}
	return &ab.BroadcastResponse{Status: f.status}, nil
}

func (f *fakeCFTStream) CloseSend() error {
	return nil
}

// TestCFTBroadcaster_NewCFTBroadcaster tests the constructor
func TestCFTBroadcaster_NewCFTBroadcaster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		poolSize    int
		networkName string
	}{
		{
			name:        "creates broadcaster with valid pool size",
			poolSize:    4,
			networkName: "test-network",
		},
		{
			name:        "creates broadcaster with pool size 1",
			poolSize:    1,
			networkName: "single-pool",
		},
		{
			name:        "creates broadcaster with large pool",
			poolSize:    100,
			networkName: "large-pool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &fake.ConfigService{
				PoolSizeValue:    tt.poolSize,
				NetworkNameValue: tt.networkName,
			}
			m := &metrics.Metrics{}

			b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, m)

			require.NotNil(t, b)
			require.Equal(t, tt.networkName, b.NetworkID)
			require.Equal(t, cfg, b.ConfigService)
			require.NotNil(t, b.ClientFactory)
			require.NotNil(t, b.connections)
			require.NotNil(t, b.connSem)
			require.Equal(t, m, b.metrics)
			require.Equal(t, tt.poolSize, cap(b.connections))
		})
	}
}

// TestCFTBroadcaster_Broadcast_Success tests successful broadcast scenarios
func TestCFTBroadcaster_Broadcast_Success(t *testing.T) {
	t.Parallel()

	t.Run("successful broadcast on first attempt", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			RetriesValue:     3,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		m := &metrics.Metrics{}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, m)

		err := b.Broadcast(t.Context(), &common.Envelope{})
		require.NoError(t, err)
	})

	t.Run("successful broadcast with pooled connection", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			RetriesValue:     3,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		m := &metrics.Metrics{}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, m)

		// First broadcast creates connection
		err := b.Broadcast(t.Context(), &common.Envelope{})
		require.NoError(t, err)

		// Second broadcast should reuse connection
		err = b.Broadcast(t.Context(), &common.Envelope{})
		require.NoError(t, err)
	})
}

// TestCFTBroadcaster_Broadcast_Failures tests failure scenarios
func TestCFTBroadcaster_Broadcast_Failures(t *testing.T) {
	t.Parallel()

	t.Run("no orderer configured", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			RetriesValue:     3,
			NetworkNameValue: "test-network",
		}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, nil)

		err := b.Broadcast(t.Context(), &common.Envelope{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "no orderer configured")
	})

	t.Run("client creation fails", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			RetriesValue:     2,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		services := &fakeCFTServices{
			shouldFailClient: true,
			clientError:      errors.New("connection refused"),
		}
		b := NewCFTBroadcaster(cfg, services, nil)

		err := b.Broadcast(t.Context(), &common.Envelope{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to send transaction to orderer")
	})

	t.Run("stream creation fails", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			RetriesValue:     2,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		services := &fakeCFTServices{
			shouldFailStream: true,
			streamError:      errors.New("stream error"),
		}
		b := NewCFTBroadcaster(cfg, services, nil)

		err := b.Broadcast(t.Context(), &common.Envelope{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to send transaction to orderer")
	})
}

// TestCFTBroadcaster_Broadcast_Retries tests retry logic
func TestCFTBroadcaster_Broadcast_Retries(t *testing.T) {
	t.Parallel()

	t.Run("succeeds on retry after send failure", func(t *testing.T) {
		t.Parallel()

		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			RetriesValue:     3,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}

		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, nil)

		// First broadcast succeeds and pools connection
		err := b.Broadcast(t.Context(), &common.Envelope{})
		require.NoError(t, err)

		// Now replace pooled connection with one that fails on send
		// Get the pooled connection
		conn := <-b.connections

		// Replace with failing stream
		conn.Stream = &fakeCFTStream{sendErr: errors.New("temporary failure")}
		b.connections <- conn

		// This broadcast will fail on first attempt (using pooled connection)
		// then retry with a new connection and succeed
		err = b.Broadcast(t.Context(), &common.Envelope{})
		require.NoError(t, err)
	})

	t.Run("fails after exhausting retries", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			RetriesValue:     2,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		services := &fakeCFTServices{
			shouldFailClient: true,
		}
		b := NewCFTBroadcaster(cfg, services, nil)

		err := b.Broadcast(t.Context(), &common.Envelope{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to send transaction to orderer")
	})
}

// TestCFTBroadcaster_GetConnection tests connection acquisition
func TestCFTBroadcaster_GetConnection(t *testing.T) {
	t.Parallel()

	t.Run("reuses pooled connection", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, nil)

		// Create and pool a connection
		conn1, err := b.getConnection(t.Context())
		require.NoError(t, err)
		require.NotNil(t, conn1)

		b.releaseConnection(conn1)

		// Get connection again - should reuse
		conn2, err := b.getConnection(t.Context())
		require.NoError(t, err)
		require.Same(t, conn1, conn2)
	})

	t.Run("creates new connection when pool empty", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, nil)

		conn, err := b.getConnection(t.Context())
		require.NoError(t, err)
		require.NotNil(t, conn)
		require.NotNil(t, conn.Stream)
		require.NotNil(t, conn.Client)
	})

	t.Run("fails when no orderer configured", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			NetworkNameValue: "test-network",
		}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, nil)

		_, err := b.getConnection(t.Context())
		require.Error(t, err)
		require.Contains(t, err.Error(), "no orderer configured")
	})

	t.Run("fails when client creation fails", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		services := &fakeCFTServices{
			shouldFailClient: true,
			clientError:      errors.New("connection error"),
		}
		b := NewCFTBroadcaster(cfg, services, nil)

		_, err := b.getConnection(t.Context())
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed creating orderer client")
	})

	t.Run("fails when stream creation fails", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		services := &fakeCFTServices{
			shouldFailStream: true,
			streamError:      errors.New("stream error"),
		}
		b := NewCFTBroadcaster(cfg, services, nil)

		_, err := b.getConnection(t.Context())
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to new a broadcast")
	})
}

// TestCFTBroadcaster_DiscardConnection tests connection cleanup
func TestCFTBroadcaster_DiscardConnection(t *testing.T) {
	t.Parallel()

	t.Run("discards connection and releases semaphore", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, nil)

		conn, err := b.getConnection(t.Context())
		require.NoError(t, err)

		// Verify semaphore is acquired
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		defer cancel()
		err = b.connSem.Acquire(ctx, 1)
		require.NoError(t, err)
		b.connSem.Release(1)

		b.discardConnection(conn)

		// After discard, semaphore should be released
		ctx2, cancel2 := context.WithTimeout(t.Context(), 10*time.Millisecond)
		defer cancel2()
		err = b.connSem.Acquire(ctx2, 1)
		require.NoError(t, err)
	})

	t.Run("handles nil connection", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			NetworkNameValue: "test-network",
		}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, nil)

		require.NotPanics(t, func() {
			b.discardConnection(nil)
		})
	})

	t.Run("handles connection with nil stream", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			NetworkNameValue: "test-network",
		}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, nil)

		// Acquire semaphore manually
		err := b.connSem.Acquire(t.Context(), 1)
		require.NoError(t, err)

		conn := &Connection{
			Stream: nil,
			Client: &fakeCFTClient{},
		}

		require.NotPanics(t, func() {
			b.discardConnection(conn)
		})
	})

	t.Run("handles connection with nil client", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			NetworkNameValue: "test-network",
		}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, nil)

		// Acquire semaphore manually
		err := b.connSem.Acquire(t.Context(), 1)
		require.NoError(t, err)

		conn := &Connection{
			Stream: &fakeCFTStream{},
			Client: nil,
		}

		require.NotPanics(t, func() {
			b.discardConnection(conn)
		})
	})
}

// TestCFTBroadcaster_ReleaseConnection tests connection pooling
func TestCFTBroadcaster_ReleaseConnection(t *testing.T) {
	t.Parallel()

	t.Run("returns connection to pool", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, nil)

		conn, err := b.getConnection(t.Context())
		require.NoError(t, err)

		initialPoolSize := len(b.connections)
		b.releaseConnection(conn)

		require.Len(t, b.connections, initialPoolSize+1)
	})

	t.Run("discards when pool full", func(t *testing.T) {
		t.Parallel()
		const poolSize = 2
		cfg := &fake.ConfigService{
			PoolSizeValue:    poolSize,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, nil)

		// Create connections and fill the pool
		var conns []*Connection
		for range poolSize {
			conn, err := b.getConnection(t.Context())
			require.NoError(t, err)
			conns = append(conns, conn)
		}

		// Release all connections to fill the pool
		for _, conn := range conns {
			b.releaseConnection(conn)
		}

		require.Len(t, b.connections, poolSize, "pool should be full")

		// Create one more connection (this acquires a semaphore slot)
		conn, err := b.getConnection(t.Context())
		require.NoError(t, err)

		// Try to release it - should be discarded because pool is full
		b.releaseConnection(conn)

		// Pool should still be at capacity (not increased)
		require.Len(t, b.connections, poolSize, "pool should still be at capacity")
	})
}

// TestCFTBroadcaster_ConcurrentBroadcasts tests concurrent operations
func TestCFTBroadcaster_ConcurrentBroadcasts(t *testing.T) {
	t.Parallel()

	t.Run("handles concurrent broadcasts", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			RetriesValue:     3,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, &metrics.Metrics{})

		const numGoroutines = 20
		errChan := make(chan error, numGoroutines)

		for range numGoroutines {
			go func() {
				err := b.Broadcast(t.Context(), &common.Envelope{})
				errChan <- err
			}()
		}

		for range numGoroutines {
			err := <-errChan
			require.NoError(t, err)
		}
	})

	t.Run("handles concurrent connection acquisition", func(t *testing.T) {
		t.Parallel()
		const poolSize = 4
		cfg := &fake.ConfigService{
			PoolSizeValue:    poolSize,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, nil)

		const numGoroutines = poolSize // Only try to get as many as pool size
		connChan := make(chan *Connection, numGoroutines)
		errChan := make(chan error, numGoroutines)

		// Use a context with timeout to prevent hanging
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		for range numGoroutines {
			go func() {
				conn, err := b.getConnection(ctx)
				if err != nil {
					errChan <- err
					return
				}
				connChan <- conn
				errChan <- nil
			}()
		}

		successCount := 0
		for range numGoroutines {
			err := <-errChan
			if err == nil {
				successCount++
			}
		}

		// All connections should succeed since we only request poolSize
		require.Equal(t, poolSize, successCount)

		// Release all acquired connections
		close(connChan)
		for conn := range connChan {
			b.releaseConnection(conn)
		}
	})
}

// TestCFTBroadcaster_ConnectionLifecycle tests full connection lifecycle
func TestCFTBroadcaster_ConnectionLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("connection lifecycle: acquire, use, release, reuse", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			RetriesValue:     3,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{}, &metrics.Metrics{})

		// First broadcast - creates connection
		err := b.Broadcast(t.Context(), &common.Envelope{})
		require.NoError(t, err)
		require.Len(t, b.connections, 1, "connection should be pooled")

		// Second broadcast - reuses connection
		err = b.Broadcast(t.Context(), &common.Envelope{})
		require.NoError(t, err)
		require.Len(t, b.connections, 1, "should still have one pooled connection")

		// Third broadcast - still reuses
		err = b.Broadcast(t.Context(), &common.Envelope{})
		require.NoError(t, err)
		require.Len(t, b.connections, 1, "should still have one pooled connection")
	})

	t.Run("connection lifecycle: acquire, fail, discard", func(t *testing.T) {
		t.Parallel()
		cfg := &fake.ConfigService{
			PoolSizeValue:    4,
			RetriesValue:     2,
			NetworkNameValue: "test-network",
			OrderersValue:    []*grpc.ConnectionConfig{{Address: "orderer-1"}},
		}
		b := NewCFTBroadcaster(cfg, &fakeCFTServices{shouldFailClient: true}, nil)

		// Broadcast should fail after exhausting retries
		// All connection attempts will fail
		err := b.Broadcast(t.Context(), &common.Envelope{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to send transaction to orderer")
		require.Len(t, b.connections, 0, "no connections should be pooled after all failures")
	})
}
