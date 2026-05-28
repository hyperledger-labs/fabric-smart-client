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

	// Pre-fill the pools and hold the matching semaphore units, so
	// getConnection bypasses ClientFactory and discard accounting balances.
	require.NoError(t, b.connSem.Acquire(context.Background(), int64(len(orderers))))
	streams := map[string]*fakeBroadcastStream{
		"o1": {status: common.Status_SUCCESS},
		"o2": {status: common.Status_SUCCESS},
		"o3": {status: common.Status_SUCCESS},
		"o4": {recvErr: errors.New("induced Recv failure")},
	}
	for _, o := range orderers {
		b.connectionPool(o.Address) <- &Connection{Stream: streams[o.Address]}
	}

	err := b.Broadcast(context.Background(), &common.Envelope{})
	require.NoError(t, err)

	// Exactly one unit should be free: o4's discarded connection.
	require.True(t, b.connSem.TryAcquire(1), "failed connection's semaphore unit should be released")
	require.False(t, b.connSem.TryAcquire(1), "only one unit should be released")
}
