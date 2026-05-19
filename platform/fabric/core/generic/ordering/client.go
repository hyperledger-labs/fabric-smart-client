/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"sync"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type Connection struct {
	lock sync.Mutex
	// Address is the orderer this connection was created for. Used by
	// BFTBroadcaster.discardConnection to release the right per-orderer slot.
	Address string
	Stream  Broadcast
	Client  Client
	Cancel  context.CancelFunc
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

	select {
	case res := <-done:
		return res.resp, res.err
	case <-ctx.Done():
		if c.Cancel != nil {
			c.Cancel()
		}
		if c.Client != nil {
			c.Client.Close()
		}
		return nil, ctx.Err()
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
