/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"crypto/tls"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	discovery2 "github.com/hyperledger/fabric/discovery/client"
	grpc2 "google.golang.org/grpc"
)

const signerCacheSize = 1

// GRPCClient represents a grpc-based client for communicating with a peer
type GRPCClient struct {
	Client  *grpc.Client
	address string
	signer  discovery2.Signer
	connect func() (*grpc2.ClientConn, error)
}

func NewGRPCClient(client *grpc.Client, address string, sn string, signer discovery2.Signer) *GRPCClient {
	return newClient(client, address, signer, func() (*grpc2.ClientConn, error) {
		return client.NewConnection(address, grpc.ServerNameOverride(sn))
	})
}

func newClient(c *grpc.Client, address string, signer discovery2.Signer, connect func() (*grpc2.ClientConn, error)) *GRPCClient {
	return &GRPCClient{
		Client:  c,
		address: address,
		signer:  signer,
		connect: connect,
	}
}

func (c *GRPCClient) EndorserClient() (pb.EndorserClient, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	return pb.NewEndorserClient(conn), nil
}

func (c *GRPCClient) DiscoveryClient() (DiscoveryClient, error) {
	return discovery2.NewClient(c.connect, c.signer, signerCacheSize), nil
}

func (c *GRPCClient) DeliverClient() (pb.DeliverClient, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	return pb.NewDeliverClient(conn), nil
}

func (c *GRPCClient) Certificate() tls.Certificate {
	return c.Client.Certificate()
}

func (c *GRPCClient) Address() string {
	return c.address
}

func (c *GRPCClient) Close() {
	c.Client.Close()
}

// lazyGRPCClient reuses the same client connection unless this connection is reset
type lazyGRPCClient struct {
	*GRPCClient
	reset func() error
}

func (c *lazyGRPCClient) Reset() error {
	return c.reset()
}

func NewLazyGRPCClient(pc *GRPCClient) *lazyGRPCClient {
	holder := lazy.NewCloserHolder(pc.connect)
	return &lazyGRPCClient{
		GRPCClient: newClient(pc.Client, pc.address, pc.signer, holder.Get),
		reset:      holder.Reset,
	}
}
