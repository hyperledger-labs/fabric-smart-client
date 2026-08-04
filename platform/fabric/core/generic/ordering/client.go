/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

// defaultRecvHardTimeout is the default value for Connection.RecvHardTimeout, used
// whenever a Connection is constructed without setting the field explicitly. It only
// applies to callers that pass a context without a deadline of their own.
const defaultRecvHardTimeout = 30 * time.Second

type Connection struct {
	lock sync.Mutex
	// Address is the orderer this connection was created for. Used by
	// BFTBroadcaster.discardConnection to release the right per-orderer slot.
	Address string
	Stream  Broadcast
	Client  Client
	Cancel  context.CancelFunc
	// RecvHardTimeout bounds how long SendAndRecv itself waits for Stream.Recv(), so a
	// caller that supplied a context without a deadline cannot block forever on a
	// network-level stall that Cancel()/Client.Close() fail to unblock.
	//
	// It bounds the call, not the goroutine: if Stream.Recv() never returns, the
	// background goroutine stays parked in it regardless. The `done` channel is
	// buffered, so that goroutine does not leak on its send once nobody is left to
	// receive; the timeout exists so the caller is not held hostage by it.
	//
	// It is only armed when ctx has no deadline of its own — a caller that set an
	// explicit deadline owns the bound. Zero falls back to defaultRecvHardTimeout.
	RecvHardTimeout time.Duration
}

func (c *Connection) Send(m *common.Envelope) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.Stream.Send(m)
}

func (c *Connection) Recv() (*ab.BroadcastResponse, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.Stream.Recv()
}

func (c *Connection) SendAndRecv(ctx context.Context, m *common.Envelope) (*ab.BroadcastResponse, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if err := c.Stream.Send(m); err != nil {
		return nil, err
	}

	type recvResult struct {
		resp *ab.BroadcastResponse
		err  error
	}
	done := make(chan recvResult, 1)
	go func() {
		resp, err := c.Stream.Recv()
		done <- recvResult{resp: resp, err: err}
	}()

	// Close()/Cancel() are expected to unblock Stream.Recv(); the hard timeout below is
	// a defensive backstop for network-level stalls where they do not. It is only armed
	// when the caller's context carries no deadline of its own, so an explicit caller
	// deadline is never silently shortened to RecvHardTimeout. See the field docs on
	// Connection.RecvHardTimeout for what this does and does not bound.
	var hardTimeout <-chan time.Time
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		d := c.RecvHardTimeout
		if d <= 0 {
			d = defaultRecvHardTimeout
		}
		timer := time.NewTimer(d)
		defer timer.Stop()
		hardTimeout = timer.C
	}

	select {
	case res := <-done:
		return res.resp, res.err
	case <-ctx.Done():
		c.abort()
		return nil, ctx.Err()
	case <-hardTimeout:
		c.abort()
		return nil, errors.New("timed out waiting for orderer Recv")
	}
}

// abort tears down the connection so a stalled Stream.Recv() has the best chance of
// returning. Safe to call on a partially populated Connection.
func (c *Connection) abort() {
	if c.Cancel != nil {
		c.Cancel()
	}
	if c.Client != nil {
		c.Client.Close()
	}
}

type Client = services.OrdererClient

type Services interface {
	NewOrdererClient(cc grpc.ConnectionConfig) (Client, error)
}

// Broadcast defines the interface that abstracts grpc calls to broadcast transactions to orderer
type Broadcast interface {
	Send(m *common.Envelope) error
	Recv() (*ab.BroadcastResponse, error)
	CloseSend() error
}
