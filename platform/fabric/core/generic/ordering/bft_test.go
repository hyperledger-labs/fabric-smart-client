/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/stretchr/testify/require"
	ggrpc "google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
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

// fakeConfigService overrides only the ConfigService methods Broadcast uses.
type fakeConfigService struct {
	driver.ConfigService
	poolSize int
	retries  int
	orderers []*grpc.ConnectionConfig
}

func (c *fakeConfigService) OrdererConnectionPoolSize() int        { return c.poolSize }
func (c *fakeConfigService) BroadcastNumRetries() int              { return c.retries }
func (c *fakeConfigService) BroadcastRetryInterval() time.Duration { return 0 }
func (c *fakeConfigService) Orderers() []*grpc.ConnectionConfig    { return c.orderers }

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

// A connection that errored mid-broadcast must be discarded even when the
// broadcast meets its BFT threshold and returns success.
func TestBFTBroadcaster_DiscardsFailedConnectionsOnPartialFailureSuccess(t *testing.T) {
	t.Parallel()

	orderers := []*grpc.ConnectionConfig{
		{Address: "o1"}, {Address: "o2"}, {Address: "o3"}, {Address: "o4"},
	}
	cfg := &fakeConfigService{
		poolSize: len(orderers),
		retries:  1,
		orderers: orderers,
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
	b := NewBFTBroadcaster(&fakeConfigService{poolSize: poolSize}, fakeServices{}, nil)
	to := &grpc.ConnectionConfig{Address: "orderer-0"}

	// Exhaust the target: create poolSize connections and keep them checked
	// out, so the next getConnection can neither reuse nor create.
	for i := 0; i < poolSize; i++ {
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
