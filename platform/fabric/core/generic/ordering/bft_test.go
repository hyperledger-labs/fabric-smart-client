/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/stretchr/testify/require"
	ggrpc "google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ordering/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

// fakeBroadcastStream is a Broadcast-interface stub with per-instance outcomes.
type fakeBroadcastStream struct {
	sendErr error
	recvErr error
	status  common.Status
}

func (f *fakeBroadcastStream) Send(*common.Envelope) error { return f.sendErr }

func (f *fakeBroadcastStream) Recv() (*ab.BroadcastResponse, error) {
	if f.recvErr != nil {
		return nil, f.recvErr
	}
	return &ab.BroadcastResponse{Status: f.status}, nil
}

func (f *fakeBroadcastStream) CloseSend() error { return nil }

// fakeServices hands out orderer clients whose stream creation is instant, so
// tests exercise the acquire/pool machinery without real network I/O.
type fakeServices struct{}

func (fakeServices) NewOrdererClient(grpc.ConnectionConfig) (Client, error) { return fakeClient{}, nil }

type fakeClient struct{ Client }

func (fakeClient) OrdererClient() (ab.AtomicBroadcastClient, error) { return fakeAB{}, nil }
func (fakeClient) Close()                                           {}

type fakeAB struct{}

func (fakeAB) Broadcast(context.Context, ...ggrpc.CallOption) (ggrpc.BidiStreamingClient[common.Envelope, ab.BroadcastResponse], error) {
	return fakeOrdererStream{}, nil
}

func (fakeAB) Deliver(context.Context, ...ggrpc.CallOption) (ggrpc.BidiStreamingClient[common.Envelope, ab.DeliverResponse], error) {
	return nil, nil
}

type fakeOrdererStream struct {
	ggrpc.BidiStreamingClient[common.Envelope, ab.BroadcastResponse]
}

func (fakeOrdererStream) Send(*common.Envelope) error { return nil }
func (fakeOrdererStream) Recv() (*ab.BroadcastResponse, error) {
	return &ab.BroadcastResponse{Status: common.Status_SUCCESS}, nil
}
func (fakeOrdererStream) CloseSend() error { return nil }

// fakeRetryServices is a stateful fake that can fail initially and succeed on retry
type fakeRetryServices struct {
	shouldFail func() bool
}

func (f *fakeRetryServices) NewOrdererClient(grpc.ConnectionConfig) (Client, error) {
	if f.shouldFail() {
		return nil, errors.New("connection failed")
	}
	return fakeClient{}, nil
}

// A connection that errored mid-broadcast must be discarded even when the
// broadcast meets its BFT threshold and returns success.
func TestBFTBroadcaster_DiscardsFailedConnectionsOnPartialFailureSuccess(t *testing.T) {
	t.Parallel()

	orderers := []*grpc.ConnectionConfig{
		{Address: "o1"}, {Address: "o2"}, {Address: "o3"}, {Address: "o4"},
	}
	cfg := &fake.ConfigService{
		PoolSizeValue: len(orderers),
		RetriesValue:  1,
		OrderersValue: orderers,
	}
	b := NewBFTBroadcaster(cfg, nil, nil)

	streams := map[string]*fakeBroadcastStream{
		"o1": {status: common.Status_SUCCESS},
		"o2": {status: common.Status_SUCCESS},
		"o3": {status: common.Status_SUCCESS},
		"o4": {recvErr: errors.New("induced Recv failure")},
	}
	// Pre-fill each orderer's pool with one connection, consuming one slot, so
	// getConnection reuses it (bypassing ClientFactory) and the per-orderer slot
	// accounting can be asserted after the broadcast.
	for _, o := range orderers {
		state := b.ordererState(o.Address)
		<-state.slots
		state.pool <- &Connection{Address: o.Address, Stream: streams[o.Address]}
	}

	err := b.Broadcast(t.Context(), &common.Envelope{})
	require.NoError(t, err)

	// o4's errored connection must be discarded, returning its slot, so o4 is
	// back to full capacity. o1-o3 succeeded and their connections went back to
	// the pool, so those slots stay held.
	require.Len(t, b.ordererState("o4").slots, len(orderers), "failed connection's slot should be released")
	for _, addr := range []string{"o1", "o2", "o3"} {
		require.Len(t, b.ordererState(addr).slots, len(orderers)-1, "pooled connection keeps holding its slot")
	}
}

// getConnection must return promptly when the caller's context is cancelled
// while no connection is obtainable (pool empty, every slot held). The acquire
// path blocks on a select that includes ctx.Done(); a regression dropping that
// case would busy-spin and miss the deadline.
func TestBFTBroadcaster_GetConnectionHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	const poolSize = 4
	b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: poolSize}, fakeServices{}, nil)
	to := &grpc.ConnectionConfig{Address: "orderer-0"}

	// Exhaust the target: create poolSize connections and keep them checked
	// out, so the next getConnection can neither reuse nor create.
	for range poolSize {
		c, err := b.getConnection(t.Context(), to)
		require.NoError(t, err)
		require.NotNil(t, c)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	done := make(chan error, 1)
	go func() {
		_, err := b.getConnection(ctx, to)
		done <- err
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("getConnection ignored a cancelled context (busy-spin)")
	}
}

// TestBFTBroadcaster_NewBFTBroadcaster tests the constructor
func TestBFTBroadcaster_NewBFTBroadcaster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		poolSize    int
		shouldPanic bool
	}{
		{
			name:        "valid pool size",
			poolSize:    4,
			shouldPanic: false,
		},
		{
			name:        "zero pool size panics",
			poolSize:    0,
			shouldPanic: true,
		},
		{
			name:        "negative pool size panics",
			poolSize:    -1,
			shouldPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &fake.ConfigService{PoolSizeValue: tt.poolSize}

			if tt.shouldPanic {
				require.Panics(t, func() {
					NewBFTBroadcaster(cfg, fakeServices{}, nil)
				})
			} else {
				b := NewBFTBroadcaster(cfg, fakeServices{}, nil)
				require.NotNil(t, b)
				require.Equal(t, tt.poolSize, b.poolSize)
				require.NotNil(t, b.states)
				require.Empty(t, b.states)
			}
		})
	}
}

// TestBFTBroadcaster_OrdererState tests orderer state initialization and caching
func TestBFTBroadcaster_OrdererState(t *testing.T) {
	t.Parallel()

	const poolSize = 4
	b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: poolSize}, fakeServices{}, nil)

	t.Run("creates new state", func(t *testing.T) {
		t.Parallel()
		state := b.ordererState("orderer-1")
		require.NotNil(t, state)
		require.NotNil(t, state.pool)
		require.NotNil(t, state.slots)
		require.Len(t, state.slots, poolSize)
	})

	t.Run("returns cached state", func(t *testing.T) {
		t.Parallel()
		state1 := b.ordererState("orderer-2")
		state2 := b.ordererState("orderer-2")
		require.Same(t, state1, state2)
	})

	t.Run("different orderers have different states", func(t *testing.T) {
		t.Parallel()
		state1 := b.ordererState("orderer-3")
		state2 := b.ordererState("orderer-4")
		require.NotSame(t, state1, state2)
	})
}

// TestBFTBroadcaster_Broadcast_Success tests successful broadcast scenarios
func TestBFTBroadcaster_Broadcast_Success(t *testing.T) {
	t.Parallel()

	t.Run("all orderers succeed", func(t *testing.T) {
		t.Parallel()
		orderers := []*grpc.ConnectionConfig{
			{Address: "o1"}, {Address: "o2"}, {Address: "o3"}, {Address: "o4"},
		}
		cfg := &fake.ConfigService{
			PoolSizeValue: 4,
			RetriesValue:  1,
			OrderersValue: orderers,
		}
		b := NewBFTBroadcaster(cfg, fakeServices{}, nil)

		err := b.Broadcast(t.Context(), &common.Envelope{})
		require.NoError(t, err)
	})

	t.Run("meets threshold with one failure", func(t *testing.T) {
		t.Parallel()
		orderers := []*grpc.ConnectionConfig{
			{Address: "o1"}, {Address: "o2"}, {Address: "o3"}, {Address: "o4"},
		}
		cfg := &fake.ConfigService{
			PoolSizeValue: 4,
			RetriesValue:  1,
			OrderersValue: orderers,
		}
		b := NewBFTBroadcaster(cfg, nil, nil)

		streams := map[string]*fakeBroadcastStream{
			"o1": {status: common.Status_SUCCESS},
			"o2": {status: common.Status_SUCCESS},
			"o3": {status: common.Status_SUCCESS},
			"o4": {recvErr: errors.New("connection failed")},
		}

		for _, o := range orderers {
			state := b.ordererState(o.Address)
			<-state.slots
			state.pool <- &Connection{Address: o.Address, Stream: streams[o.Address]}
		}

		err := b.Broadcast(t.Context(), &common.Envelope{})
		require.NoError(t, err)
	})

	t.Run("meets threshold with f failures", func(t *testing.T) {
		t.Parallel()
		orderers := []*grpc.ConnectionConfig{
			{Address: "o1"},
			{Address: "o2"},
			{Address: "o3"},
			{Address: "o4"},
			{Address: "o5"},
			{Address: "o6"},
			{Address: "o7"},
		}
		cfg := &fake.ConfigService{
			PoolSizeValue: 7,
			RetriesValue:  1,
			OrderersValue: orderers,
		}
		b := NewBFTBroadcaster(cfg, nil, nil)

		// n=7, f=2, threshold=5
		// 5 successes, 2 failures should succeed
		streams := map[string]*fakeBroadcastStream{
			"o1": {status: common.Status_SUCCESS},
			"o2": {status: common.Status_SUCCESS},
			"o3": {status: common.Status_SUCCESS},
			"o4": {status: common.Status_SUCCESS},
			"o5": {status: common.Status_SUCCESS},
			"o6": {recvErr: errors.New("failed")},
			"o7": {recvErr: errors.New("failed")},
		}

		for _, o := range orderers {
			state := b.ordererState(o.Address)
			<-state.slots
			state.pool <- &Connection{Address: o.Address, Stream: streams[o.Address]}
		}

		err := b.Broadcast(t.Context(), &common.Envelope{})
		require.NoError(t, err)
	})
}

// TestBFTBroadcaster_Broadcast_Failures tests failure scenarios
func TestBFTBroadcaster_Broadcast_Failures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		orderers     []*grpc.ConnectionConfig
		streams      map[string]*fakeBroadcastStream
		retries      int
		errorMessage string
	}{
		{
			name: "not enough orderers",
			orderers: []*grpc.ConnectionConfig{
				{Address: "o1"}, {Address: "o2"}, {Address: "o3"},
			},
			streams:      nil,
			retries:      1,
			errorMessage: "not enough orderers",
		},
		{
			name: "below threshold failures",
			orderers: []*grpc.ConnectionConfig{
				{Address: "o1"}, {Address: "o2"}, {Address: "o3"}, {Address: "o4"},
			},
			streams: map[string]*fakeBroadcastStream{
				"o1": {status: common.Status_SUCCESS},
				"o2": {status: common.Status_SUCCESS},
				"o3": {recvErr: errors.New("failed")},
				"o4": {recvErr: errors.New("failed")},
			},
			retries:      1,
			errorMessage: "failed to send transaction to the ordering service",
		},
		{
			name: "all orderers fail",
			orderers: []*grpc.ConnectionConfig{
				{Address: "o1"}, {Address: "o2"}, {Address: "o3"}, {Address: "o4"},
			},
			streams: map[string]*fakeBroadcastStream{
				"o1": {recvErr: errors.New("failed")},
				"o2": {recvErr: errors.New("failed")},
				"o3": {recvErr: errors.New("failed")},
				"o4": {recvErr: errors.New("failed")},
			},
			retries:      1,
			errorMessage: "failed to send transaction to the ordering service",
		},
		{
			name: "non-success status codes",
			orderers: []*grpc.ConnectionConfig{
				{Address: "o1"}, {Address: "o2"}, {Address: "o3"}, {Address: "o4"},
			},
			streams: map[string]*fakeBroadcastStream{
				"o1": {status: common.Status_SUCCESS},
				"o2": {status: common.Status_BAD_REQUEST},
				"o3": {status: common.Status_FORBIDDEN},
				"o4": {status: common.Status_INTERNAL_SERVER_ERROR},
			},
			retries:      1,
			errorMessage: "failed to send transaction to the ordering service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &fake.ConfigService{
				PoolSizeValue: len(tt.orderers),
				RetriesValue:  tt.retries,
				OrderersValue: tt.orderers,
			}

			b := NewBFTBroadcaster(cfg, fakeServices{}, nil)

			// Pre-populate pool with custom streams if provided
			if tt.streams != nil {
				for _, o := range tt.orderers {
					state := b.ordererState(o.Address)
					<-state.slots
					state.pool <- &Connection{Address: o.Address, Stream: tt.streams[o.Address]}
				}
			}

			err := b.Broadcast(t.Context(), &common.Envelope{})
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.errorMessage)
		})
	}
}

// TestBFTBroadcaster_Broadcast_Retries tests retry logic
func TestBFTBroadcaster_Broadcast_Retries(t *testing.T) {
	t.Parallel()

	t.Run("succeeds on retry after initial failures", func(t *testing.T) {
		t.Parallel()
		orderers := []*grpc.ConnectionConfig{
			{Address: "o1"}, {Address: "o2"}, {Address: "o3"}, {Address: "o4"},
		}
		cfg := &fake.ConfigService{
			PoolSizeValue: 4,
			RetriesValue:  3,
			OrderersValue: orderers,
		}

		// Create a stateful fake service that fails on first attempt, succeeds on retry
		failCount := 0
		var mu sync.Mutex
		retryServices := &fakeRetryServices{
			shouldFail: func() bool {
				mu.Lock()
				defer mu.Unlock()
				failCount++
				// Fail first 4 connection attempts (one per orderer), succeed on retry
				return failCount <= 4
			},
		}

		b := NewBFTBroadcaster(cfg, retryServices, nil)

		// First attempt will fail (all connections fail), second will succeed
		err := b.Broadcast(t.Context(), &common.Envelope{})
		require.NoError(t, err)

		// Verify we actually retried beyond the initial round of 4 connection attempts
		require.Greater(t, failCount, 4, "should have retried after the initial failed connection attempts")
	})
}

// TestBFTBroadcaster_GetConnection tests connection acquisition
func TestBFTBroadcaster_GetConnection(t *testing.T) {
	t.Parallel()

	t.Run("reuses pooled connection", func(t *testing.T) {
		t.Parallel()
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: 4}, fakeServices{}, nil)
		to := &grpc.ConnectionConfig{Address: "orderer-1"}

		// Create and pool a connection
		conn1, err := b.getConnection(t.Context(), to)
		require.NoError(t, err)
		b.releaseConnection(conn1, to)

		// Get connection again - should reuse
		conn2, err := b.getConnection(t.Context(), to)
		require.NoError(t, err)
		require.Same(t, conn1, conn2)
	})

	t.Run("creates new connection when pool empty", func(t *testing.T) {
		t.Parallel()
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: 4}, fakeServices{}, nil)
		to := &grpc.ConnectionConfig{Address: "orderer-2"}

		conn, err := b.getConnection(t.Context(), to)
		require.NoError(t, err)
		require.NotNil(t, conn)
		require.Equal(t, "orderer-2", conn.Address)
	})

	t.Run("blocks when pool and slots exhausted", func(t *testing.T) {
		t.Parallel()
		const poolSize = 2
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: poolSize}, fakeServices{}, nil)
		to := &grpc.ConnectionConfig{Address: "orderer-3"}

		// Exhaust all slots
		for range poolSize {
			_, err := b.getConnection(t.Context(), to)
			require.NoError(t, err)
		}

		// Next call should block until context cancelled
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()

		_, err := b.getConnection(ctx, to)
		require.Error(t, err)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

// TestBFTBroadcaster_CreateConnection tests connection creation
func TestBFTBroadcaster_CreateConnection(t *testing.T) {
	t.Parallel()

	t.Run("creates valid connection", func(t *testing.T) {
		t.Parallel()
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: 4}, fakeServices{}, nil)
		to := &grpc.ConnectionConfig{Address: "orderer-1"}

		conn, err := b.createConnection(to)
		require.NoError(t, err)
		require.NotNil(t, conn)
		require.Equal(t, "orderer-1", conn.Address)
		require.NotNil(t, conn.Stream)
		require.NotNil(t, conn.Client)
		require.NotNil(t, conn.Cancel)
	})
}

// TestBFTBroadcaster_DiscardConnection tests connection cleanup
func TestBFTBroadcaster_DiscardConnection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupConn  func(*testing.T, *BFTBroadcaster) *Connection
		checkPanic bool
	}{
		{
			name: "discards connection and releases slot",
			setupConn: func(t *testing.T, b *BFTBroadcaster) *Connection {
				t.Helper()
				to := &grpc.ConnectionConfig{Address: "orderer-1"}
				conn, err := b.getConnection(t.Context(), to)
				require.NoError(t, err)
				return conn
			},
			checkPanic: false,
		},
		{
			name: "handles nil connection",
			setupConn: func(t *testing.T, b *BFTBroadcaster) *Connection {
				t.Helper()
				return nil
			},
			checkPanic: false,
		},
		{
			name: "handles connection with nil fields",
			setupConn: func(t *testing.T, b *BFTBroadcaster) *Connection {
				t.Helper()
				// Pre-allocate a slot so we can verify it gets released
				state := b.ordererState("orderer-1")
				<-state.slots
				return &Connection{Address: "orderer-1"}
			},
			checkPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			const poolSize = 4
			b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: poolSize}, fakeServices{}, nil)
			conn := tt.setupConn(t, b)

			var slotsBeforeDiscard int
			if conn != nil && conn.Address != "" {
				state := b.ordererState(conn.Address)
				slotsBeforeDiscard = len(state.slots)
			}

			require.NotPanics(t, func() {
				b.discardConnection(conn)
			})

			if conn != nil && conn.Address != "" {
				state := b.ordererState(conn.Address)
				expectedSlotsAfterDiscard := slotsBeforeDiscard + 1
				require.Len(t, state.slots, expectedSlotsAfterDiscard, "slot should be released after discard")
			}
		})
	}
}

// TestBFTBroadcaster_ReleaseConnection tests connection pooling
func TestBFTBroadcaster_ReleaseConnection(t *testing.T) {
	t.Parallel()

	t.Run("returns connection to pool", func(t *testing.T) {
		t.Parallel()
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: 4}, fakeServices{}, nil)
		to := &grpc.ConnectionConfig{Address: "orderer-1"}

		conn, err := b.getConnection(t.Context(), to)
		require.NoError(t, err)

		state := b.ordererState(to.Address)
		initialPoolSize := len(state.pool)

		b.releaseConnection(conn, to)

		require.Len(t, state.pool, initialPoolSize+1)
	})

	t.Run("discards when pool full", func(t *testing.T) {
		t.Parallel()
		const poolSize = 2
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: poolSize}, fakeServices{}, nil)
		to := &grpc.ConnectionConfig{Address: "orderer-1"}

		state := b.ordererState(to.Address)

		// Fill the pool by creating connections and releasing them
		for range poolSize {
			conn, err := b.createConnection(to)
			require.NoError(t, err)
			// Take a slot first
			<-state.slots
			state.pool <- conn
		}

		require.Len(t, state.pool, poolSize, "pool should be full")
		slotsBeforeRelease := len(state.slots)

		// Try to release one more - should be discarded because pool is full
		conn, err := b.createConnection(to)
		require.NoError(t, err)
		b.releaseConnection(conn, to)

		// Pool should still be at capacity (not increased)
		require.Len(t, state.pool, poolSize, "pool should still be at capacity")
		// Slot should be released when connection is discarded
		require.Len(t, state.slots, slotsBeforeRelease+1, "discarded connection's slot should be released")
	})
}

// TestBFTBroadcaster_ReleaseSlot tests slot management
func TestBFTBroadcaster_ReleaseSlot(t *testing.T) {
	t.Parallel()

	t.Run("releases slot successfully", func(t *testing.T) {
		t.Parallel()
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: 4}, fakeServices{}, nil)
		state := b.ordererState("orderer-1")

		// Take a slot
		<-state.slots
		initialSlots := len(state.slots)

		// Release it
		b.releaseSlot(state)

		require.Len(t, state.slots, initialSlots+1)
	})

	t.Run("handles full slot channel", func(t *testing.T) {
		t.Parallel()
		const poolSize = 2
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: poolSize}, fakeServices{}, nil)
		state := b.ordererState("orderer-1")

		// Slots should already be full
		require.Len(t, state.slots, poolSize)

		// Try to release another - should not panic
		require.NotPanics(t, func() {
			b.releaseSlot(state)
		})

		// Should still be at capacity
		require.Len(t, state.slots, poolSize)
	})
}

// TestBFTBroadcaster_ThresholdCalculation tests BFT threshold logic
func TestBFTBroadcaster_ThresholdCalculation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		n         int
		f         int
		threshold int
	}{
		{"4 nodes", 4, 1, 3},
		{"7 nodes", 7, 2, 5},
		{"10 nodes", 10, 3, 7},
		{"13 nodes", 13, 4, 9},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Verify f calculation: f = (n - 1) / 3
			f := (tc.n - 1) / 3
			require.Equal(t, tc.f, f, "f calculation")

			// Verify threshold calculation using the exact production formula:
			// threshold = ceil((n + f + 1) / 2)
			threshold := int(math.Ceil((float64(tc.n) + float64(f) + 1) / 2.0))
			require.Equal(t, tc.threshold, threshold, "threshold calculation")
		})
	}
}

// TestBFTBroadcaster_ConcurrentBroadcasts tests concurrent broadcast operations
func TestBFTBroadcaster_ConcurrentBroadcasts(t *testing.T) {
	t.Parallel()

	orderers := []*grpc.ConnectionConfig{
		{Address: "o1"}, {Address: "o2"}, {Address: "o3"}, {Address: "o4"},
	}
	cfg := &fake.ConfigService{
		PoolSizeValue: 4,
		RetriesValue:  1,
		OrderersValue: orderers,
	}
	b := NewBFTBroadcaster(cfg, fakeServices{}, nil)

	const numGoroutines = 10
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
}
