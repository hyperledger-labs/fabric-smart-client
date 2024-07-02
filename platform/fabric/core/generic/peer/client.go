/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"context"
	"crypto/tls"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	discovery2 "github.com/hyperledger/fabric/discovery/client"
	"github.com/pkg/errors"
	grpc2 "google.golang.org/grpc"
)

// GRPCClient represents a grpc-based client for communicating with a peer
type GRPCClient struct {
	*grpc.Client
	PeerAddress string
	Sn          string
	Signer      discovery2.Signer
}

func (c *GRPCClient) Close() {
	c.Client.Close()
}

func (c *GRPCClient) Connection() (*grpc2.ClientConn, error) {
	conn, err := c.Client.NewConnection(c.Address(), grpc.ServerNameOverride(c.Sn))
	if err != nil {
		return nil, errors.WithMessagef(err, "endorser GRPCClient failed to connect to %s", c.Address())
	}
	return conn, nil
}

func (c *GRPCClient) EndorserClient() (pb.EndorserClient, error) {
	conn, err := c.Client.NewConnection(c.Address(), grpc.ServerNameOverride(c.Sn))
	if err != nil {
		return nil, errors.WithMessagef(err, "endorser GRPCClient failed to connect to %s", c.Address())
	}
	return pb.NewEndorserClient(conn), nil
}

func (c *GRPCClient) DiscoveryClient() (DiscoveryClient, error) {
	return discovery2.NewClient(
		func() (*grpc2.ClientConn, error) {
			conn, err := c.Client.NewConnection(c.Address(), grpc.ServerNameOverride(c.Sn))
			if err != nil {
				return nil, errors.WithMessagef(err, "discovery GRPCClient failed to connect to %s", c.Address())
			}
			return conn, nil
		},
		c.Signer,
		1), nil
}

func (c *GRPCClient) DeliverClient() (pb.DeliverClient, error) {
	conn, err := c.Client.NewConnection(c.Address(), grpc.ServerNameOverride(c.Sn))
	if err != nil {
		return nil, errors.WithMessagef(err, "endorser GRPCClient failed to connect to %s", c.Address())
	}
	return pb.NewDeliverClient(conn), nil
}

func (c *GRPCClient) Deliver() (pb.Deliver_DeliverClient, error) {
	conn, err := c.Client.NewConnection(c.Address(), grpc.ServerNameOverride(c.Sn))
	if err != nil {
		return nil, errors.WithMessagef(err, "deliver GRPCClient failed to connect to %s", c.Address())
	}
	return pb.NewDeliverClient(conn).Deliver(context.TODO())
}

func (c *GRPCClient) Certificate() tls.Certificate {
	return c.Client.Certificate()
}

func (c *GRPCClient) Address() string {
	return c.PeerAddress
}
