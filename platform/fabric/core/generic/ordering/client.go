/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"crypto/tls"
	"io"
	"strings"
	"sync"

	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

type Connection struct {
	lock   sync.Mutex
	Stream Broadcast
	Client *Client
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

//go:generate counterfeiter -o mock/Broadcaster.go -fake-name Broadcast . Broadcast

// Broadcast defines the interface that abstracts grpc calls to broadcast transactions to orderer
type Broadcast interface {
	Send(m *common.Envelope) error
	Recv() (*ab.BroadcastResponse, error)
	CloseSend() error
}

//go:generate counterfeiter -o mock/ordererclient.go -fake-name OrdererClient . OrdererClient

// OrdererClient defines the interface to create a Broadcast
type OrdererClient interface {
	// NewBroadcast returns a Broadcast
	NewBroadcast(ctx context.Context, opts ...grpc.CallOption) (Broadcast, error)

	// Certificate returns tls certificate for the orderer client
	Certificate() *tls.Certificate

	Close()
}

// Client implements OrdererClient interface
type Client struct {
	ordererAddr        string
	serverNameOverride string
	grpcClient         *grpc2.Client
	conn               *grpc.ClientConn
}

func NewClient(config *grpc2.ConnectionConfig) (*Client, error) {
	grpcClient, err := grpc2.CreateGRPCClient(config)
	if err != nil {
		err = errors.WithMessagef(err, "failed to create a Client to orderer %s", config.Address)
		return nil, err
	}
	conn, err := grpcClient.NewConnection(config.Address)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to connect to orderer %s", config.Address)
	}

	return &Client{
		ordererAddr:        config.Address,
		serverNameOverride: config.ServerNameOverride,
		grpcClient:         grpcClient,
		conn:               conn,
	}, nil
}

func (oc *Client) Close() {
	go oc.grpcClient.Close()
}

// NewBroadcast creates a Broadcast
func (oc *Client) NewBroadcast(ctx context.Context, opts ...grpc.CallOption) (Broadcast, error) {
	// reuse the existing connection to create Broadcast client
	broadcast, err := ab.NewAtomicBroadcastClient(oc.conn).Broadcast(ctx)
	if err == nil {
		return broadcast, nil
	}

	// error occurred with the existing connection, so create a new connection to orderer
	oc.conn, err = oc.grpcClient.NewConnection(oc.ordererAddr)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to connect to orderer %s", oc.ordererAddr)
	}

	// create a new Broadcast
	broadcast, err = ab.NewAtomicBroadcastClient(oc.conn).Broadcast(ctx)
	if err != nil {
		rpcStatus, _ := status.FromError(err)
		return nil, errors.Wrapf(err, "failed to new a broadcast, rpcStatus=%+v", rpcStatus)
	}
	return broadcast, nil
}

func (oc *Client) Certificate() *tls.Certificate {
	cert := oc.grpcClient.Certificate()
	return &cert
}

// BroadcastSend sends transaction envelope to orderer Service
func BroadcastSend(broadcast Broadcast, envelope *common.Envelope) error {
	return broadcast.Send(envelope)
}

// BroadcastReceive waits until it receives the response from broadcast stream
func BroadcastReceive(broadcast Broadcast, addr string, responses chan common.Status, errs chan error) {
	for {
		broadcastResponse, err := broadcast.Recv()
		if err == io.EOF {
			close(responses)
			return
		}

		if err != nil {
			rpcStatus, _ := status.FromError(err)
			errs <- errors.Wrapf(err, "broadcast recv error from orderer %s, rpcStatus=%+v", addr, rpcStatus)
			close(responses)
			return
		}

		if broadcastResponse.Status == common.Status_SUCCESS {
			responses <- broadcastResponse.Status
		} else {
			errs <- errors.Errorf("broadcast response error %d from orderer %s", int32(broadcastResponse.Status), addr)
		}
	}
}

// BroadcastWaitForResponse reads from response and errs chans until responses chan is closed
func BroadcastWaitForResponse(responses chan common.Status, errs chan error) (common.Status, error) {
	var st common.Status
	allErrs := make([]error, 0)

read:
	for {
		select {
		case s, ok := <-responses:
			if !ok {
				break read
			}
			st = s
		case e := <-errs:
			allErrs = append(allErrs, e)
		}
	}

	// drain remaining errors
	for i := 0; i < len(errs); i++ {
		e := <-errs
		allErrs = append(allErrs, e)
	}
	// close errs channel since we have read all of them
	close(errs)
	return st, toError(allErrs)
}

// toError converts []error to error
func toError(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}

	errmsgs := []string{"Multiple errors occurred in order broadcast stream: "}
	for _, err := range errs {
		errmsgs = append(errmsgs, err.Error())
	}
	return errors.New(strings.Join(errmsgs, "\n"))
}
