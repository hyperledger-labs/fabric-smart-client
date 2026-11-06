/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/stretchr/testify/require"
	ggrpc "google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ordering/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate/connstatetest"
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

func (*fakeBroadcastStream) CloseSend() error { return nil }

// gatedStream answers Recv with status once release is closed, and counts
// CloseSend calls.
type gatedStream struct {
	release    chan struct{}
	status     common.Status
	closeCalls atomic.Int32
}

func (*gatedStream) Send(*common.Envelope) error { return nil }

func (g *gatedStream) Recv() (*ab.BroadcastResponse, error) {
	<-g.release
	return &ab.BroadcastResponse{Status: g.status}, nil
}

func (g *gatedStream) CloseSend() error {
	g.closeCalls.Add(1)
	return nil
}

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
	synctest.Test(t, func(t *testing.T) {
		failing := &fakeBroadcastStream{recvErr: errors.New("induced Recv failure")}
		b, _ := newPrefilledBFT(4, nil, accepting(), accepting(), accepting(), failing)

		require.NoError(t, b.Broadcast(t.Context(), &common.Envelope{}))
		synctest.Wait()

		// o4's errored connection is discarded, returning its slot. o1-o3 put
		// their connections back in the pool, so those slots stay held.
		require.Len(t, b.ordererState("o4").slots, 4, "failed connection's slot should be released")
		for _, addr := range []string{"o1", "o2", "o3"} {
			require.Len(t, b.ordererState(addr).slots, 3, "pooled connection keeps holding its slot")
		}
	})
}

// newPrefilledBFT returns a BFT broadcaster for one orderer per stream. Each
// orderer's pool holds one connection on its stream, which takes one of its
// poolSize slots, so Broadcast reuses it instead of asking services.
func newPrefilledBFT(poolSize int, services Services, streams ...Broadcast) (*BFTBroadcaster, []*grpc.ConnectionConfig) {
	orderers := make([]*grpc.ConnectionConfig, len(streams))
	for i := range streams {
		orderers[i] = &grpc.ConnectionConfig{Address: fmt.Sprintf("o%d", i+1)}
	}
	cfg := &fake.ConfigService{PoolSizeValue: poolSize, RetriesValue: 1, OrderersValue: orderers}
	b := NewBFTBroadcaster(cfg, services, newTestMetrics())
	for i, o := range orderers {
		state := b.ordererState(o.Address)
		<-state.slots
		state.pool <- &Connection{Address: o.Address, Stream: streams[i]}
	}
	return b, orderers
}

func accepting() Broadcast { return &fakeBroadcastStream{status: common.Status_SUCCESS} }

// The tests below run in a synctest bubble: synctest.Wait returns once every
// send still running after Broadcast returned is blocked or done, and
// bftSendRecvTimeout passes on the bubble's fake clock.

// A hung orderer does not delay Broadcast once the other orderers meet the
// threshold. Its send times out after bftSendRecvTimeout, which closes its
// stream once and releases its slot.
func TestBFTBroadcaster_Broadcast_ReturnsWithoutHungOrderer(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		hung := &blockingStream{done: make(chan struct{})}
		t.Cleanup(func() { close(hung.done) })
		b, _ := newPrefilledBFT(4, nil, accepting(), accepting(), accepting(), hung)

		start := time.Now()
		require.NoError(t, b.Broadcast(t.Context(), &common.Envelope{}))
		require.Less(t, time.Since(start), bftSendRecvTimeout, "Broadcast must not wait for the hung orderer")

		o4 := b.ordererState("o4")
		synctest.Wait()
		require.Len(t, o4.slots, 3, "the hung send still holds its slot")

		time.Sleep(bftSendRecvTimeout)
		synctest.Wait()
		require.Len(t, o4.slots, 4, "the timed-out send releases its slot")
		require.Empty(t, o4.pool)
		hung.mu.Lock()
		defer hung.mu.Unlock()
		require.Equal(t, 1, hung.closeCalls, "the hung stream is closed exactly once")
	})
}

// An orderer that answers after Broadcast returned puts its connection back in
// the pool, even though the caller cancelled its context right after Broadcast
// returned.
func TestBFTBroadcaster_Broadcast_SlowOrdererReturnsToPool(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		slow := &gatedStream{release: make(chan struct{}), status: common.Status_SUCCESS}
		b, _ := newPrefilledBFT(4, nil, accepting(), accepting(), accepting(), slow)

		ctx, cancel := context.WithCancel(t.Context())
		require.NoError(t, b.Broadcast(ctx, &common.Envelope{}))
		cancel()

		o4 := b.ordererState("o4")
		synctest.Wait()
		require.Empty(t, o4.pool, "the slow send is still running")

		close(slow.release)
		synctest.Wait()
		require.Len(t, o4.pool, 1, "the slow orderer's connection returns to the pool")
		require.Zero(t, slow.closeCalls.Load(), "the slow orderer's connection is not discarded")
	})
}

// A retry makes progress while sends of the failed attempt still hold slots: a
// getConnection that waits for a held slot resumes once that send ends.
func TestBFTBroadcaster_Broadcast_RetryProgressesWhileSlotsHeld(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		// o1 and o2 reject once the test releases them, ending the first
		// attempt. o3 and o4 hang and keep holding their only slot. The retry
		// opens new connections, which accept.
		reject1 := &gatedStream{release: make(chan struct{}), status: common.Status_BAD_REQUEST}
		reject2 := &gatedStream{release: make(chan struct{}), status: common.Status_BAD_REQUEST}
		hung3 := &blockingStream{done: make(chan struct{})}
		hung4 := &blockingStream{done: make(chan struct{})}
		t.Cleanup(func() { close(hung4.done) })
		b, _ := newPrefilledBFT(1, fakeServices{}, reject1, reject2, hung3, hung4)
		b.ConfigService.(*fake.ConfigService).RetriesValue = 2

		done := make(chan error, 1)
		go func() { done <- b.Broadcast(t.Context(), &common.Envelope{}) }()

		// Every orderer holds the envelope before the first attempt fails, so
		// no send of the first attempt can start during the retry.
		synctest.Wait()
		close(reject1.release)
		close(reject2.release)

		synctest.Wait()
		require.Empty(t, done, "the retry needs o3 or o4, whose slots are held")

		close(hung3.done)
		synctest.Wait()
		require.Len(t, done, 1, "the retry resumes once o3's slot is released")
		require.NoError(t, <-done)
	})
}

// A goroutine that still waits for a slot when Broadcast returns gives up, so
// it cannot take a slot that frees later and send the envelope late.
func TestBFTBroadcaster_Broadcast_WaitingSendExitsAfterReturn(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		b, _ := newPrefilledBFT(1, fakeServices{}, accepting(), accepting(), accepting(), accepting())
		// Check out o4's connection: its only slot stays held and its pool is empty.
		o4 := b.ordererState("o4")
		<-o4.pool

		require.NoError(t, b.Broadcast(t.Context(), &common.Envelope{}))
		synctest.Wait()

		o4.slots <- struct{}{}
		synctest.Wait()
		require.Len(t, o4.slots, 1, "no goroutine took o4's freed slot")
		require.Empty(t, o4.pool, "no connection to o4 was opened")
	})
}

// Starting with no connections, every orderer gets the envelope, also the ones
// whose send starts after Broadcast returned.
func TestBFTBroadcaster_Broadcast_ColdStartReachesEveryOrderer(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		orderers := []*grpc.ConnectionConfig{{Address: "o1"}, {Address: "o2"}, {Address: "o3"}, {Address: "o4"}}
		cfg := &fake.ConfigService{PoolSizeValue: 1, RetriesValue: 1, OrderersValue: orderers}
		b := NewBFTBroadcaster(cfg, fakeServices{}, newTestMetrics())

		require.NoError(t, b.Broadcast(t.Context(), &common.Envelope{}))
		synctest.Wait()
		for _, o := range orderers {
			require.Len(t, b.ordererState(o.Address).pool, 1, "%s accepted the envelope and pooled its connection", o.Address)
		}
	})
}

// After the attempt ended, getConnection still reuses an idle connection or
// opens one on a free slot. Only waiting for a slot is skipped.
func TestBFTBroadcaster_GetConnectionAfterAttemptEnded(t *testing.T) {
	t.Parallel()
	to := &grpc.ConnectionConfig{Address: "o1"}
	ended := func(t *testing.T) context.Context {
		t.Helper()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		return ctx
	}

	t.Run("reuses an idle connection", func(t *testing.T) {
		t.Parallel()
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: 1}, fakeServices{}, newTestMetrics())
		idle, err := b.getConnection(t.Context(), to)
		require.NoError(t, err)
		b.releaseConnection(idle, to)

		c, err := b.getConnection(ended(t), to)
		require.NoError(t, err)
		require.Same(t, idle, c)
	})

	t.Run("opens a connection on a free slot", func(t *testing.T) {
		t.Parallel()
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: 1}, fakeServices{}, newTestMetrics())

		c, err := b.getConnection(ended(t), to)
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("does not wait for a held slot", func(t *testing.T) {
		t.Parallel()
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: 1}, fakeServices{}, newTestMetrics())
		_, err := b.getConnection(t.Context(), to)
		require.NoError(t, err)

		_, err = b.getConnection(ended(t), to)
		require.ErrorIs(t, err, context.Canceled)
	})
}

// getConnection must return promptly when the caller's context is cancelled
// while no connection is obtainable (pool empty, every slot held). The acquire
// path blocks on a select that includes ctx.Done(); a regression dropping that
// case would busy-spin and miss the deadline.
func TestBFTBroadcaster_GetConnectionHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	const poolSize = 4
	b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: poolSize}, fakeServices{}, newTestMetrics())
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
					NewBFTBroadcaster(cfg, fakeServices{}, newTestMetrics())
				})
			} else {
				b := NewBFTBroadcaster(cfg, fakeServices{}, newTestMetrics())
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
	b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: poolSize}, fakeServices{}, newTestMetrics())

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
		b := NewBFTBroadcaster(cfg, fakeServices{}, newTestMetrics())

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
		b := NewBFTBroadcaster(cfg, nil, newTestMetrics())

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
		b := NewBFTBroadcaster(cfg, nil, newTestMetrics())

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

			b := NewBFTBroadcaster(cfg, fakeServices{}, newTestMetrics())

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
		var failCount atomic.Int32
		retryServices := &fakeRetryServices{
			shouldFail: func() bool {
				// Fail first 4 connection attempts (one per orderer), succeed on retry
				return failCount.Add(1) <= 4
			},
		}

		b := NewBFTBroadcaster(cfg, retryServices, newTestMetrics())

		// First attempt will fail (all connections fail), second will succeed
		err := b.Broadcast(t.Context(), &common.Envelope{})
		require.NoError(t, err)

		// Verify we actually retried beyond the initial round of 4 connection attempts
		require.Greater(t, failCount.Load(), int32(4), "should have retried after the initial failed connection attempts")
	})
}

// TestBFTBroadcaster_GetConnection tests connection acquisition
func TestBFTBroadcaster_GetConnection(t *testing.T) {
	t.Parallel()

	t.Run("reuses pooled connection", func(t *testing.T) {
		t.Parallel()
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: 4}, fakeServices{}, newTestMetrics())
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
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: 4}, fakeServices{}, newTestMetrics())
		to := &grpc.ConnectionConfig{Address: "orderer-2"}

		conn, err := b.getConnection(t.Context(), to)
		require.NoError(t, err)
		require.NotNil(t, conn)
		require.Equal(t, "orderer-2", conn.Address)
	})

	t.Run("blocks when pool and slots exhausted", func(t *testing.T) {
		t.Parallel()
		const poolSize = 2
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: poolSize}, fakeServices{}, newTestMetrics())
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
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: 4}, fakeServices{}, newTestMetrics())
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
			setupConn: func(t *testing.T, _ *BFTBroadcaster) *Connection {
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
			b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: poolSize}, fakeServices{}, newTestMetrics())
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
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: 4}, fakeServices{}, newTestMetrics())
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
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: poolSize}, fakeServices{}, newTestMetrics())
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
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: 4}, fakeServices{}, newTestMetrics())
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
		b := NewBFTBroadcaster(&fake.ConfigService{PoolSizeValue: poolSize}, fakeServices{}, newTestMetrics())
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
	b := NewBFTBroadcaster(cfg, fakeServices{}, newTestMetrics())

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

// Both tests below wait in a synctest bubble for the sends that finish after
// Broadcast returned, so each Broadcast starts with every connection pooled.

func TestBFTBroadcast_BrokenConnectionReportsReconnecting(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		orderers := []*grpc.ConnectionConfig{{Address: "o1"}, {Address: "o2"}, {Address: "o3"}, {Address: "o4"}}
		cfg := &fake.ConfigService{PoolSizeValue: 1, RetriesValue: 1, OrderersValue: orderers, NetworkNameValue: "test-network"}
		connState, rec := connstatetest.NewMetrics()
		m := newTestMetrics()
		m.ConnState = connState
		b := NewBFTBroadcaster(cfg, fakeServices{}, m)

		require.NoError(t, b.Broadcast(t.Context(), &common.Envelope{}))
		synctest.Wait()
		conn := <-b.ordererState("o1").pool
		conn.Stream = &fakeBroadcastStream{sendErr: errors.New("orderer went away")}
		b.ordererState("o1").pool <- conn

		// Three of four orderers still meet the threshold.
		require.NoError(t, b.Broadcast(t.Context(), &common.Envelope{}))
		synctest.Wait()
		last, _ := rec.Last("o1")
		require.Equal(t, connstate.Reconnecting, last)
		// connect, then one accepted envelope per broadcast
		require.Equal(t, []connstate.State{connstate.Connected, connstate.Connected, connstate.Connected}, rec.States("o2"))
	})
}

func TestBFTBroadcast_CountsOrderedTransactions(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		orderers := []*grpc.ConnectionConfig{{Address: "o1"}, {Address: "o2"}, {Address: "o3"}, {Address: "o4"}}
		cfg := &fake.ConfigService{PoolSizeValue: 1, RetriesValue: 1, OrderersValue: orderers, NetworkNameValue: "test-network"}
		counter := &mock.Counter{}
		counter.WithReturns(counter)
		b := NewBFTBroadcaster(cfg, fakeServices{}, &metrics.Metrics{OrderedTransactions: counter})

		require.NoError(t, b.Broadcast(t.Context(), &common.Envelope{}))
		synctest.Wait()
		require.Equal(t, 1, counter.AddCallCount(), "one envelope, counted once however many orderers accepted it")
		require.Equal(t, []string{"network", "test-network"}, counter.WithArgsForCall(0))

		// Below the threshold the envelope is not ordered and not counted.
		for _, o := range orderers[:2] {
			conn := <-b.ordererState(o.Address).pool
			conn.Stream = &fakeBroadcastStream{status: common.Status_SERVICE_UNAVAILABLE}
			b.ordererState(o.Address).pool <- conn
		}
		require.Error(t, b.Broadcast(t.Context(), &common.Envelope{}))
		require.Equal(t, 1, counter.AddCallCount())
	})
}
