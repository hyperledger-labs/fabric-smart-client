/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger/fabric-protos-go/discovery"
	"github.com/hyperledger/fabric-protos-go/peer"
	dclient "github.com/hyperledger/fabric/discovery/client"
	"github.com/pkg/errors"
	ggrpc "google.golang.org/grpc"
)

type StatefulClient struct {
	peer.EndorserClient
	discovery.DiscoveryClient
	DC            DiscoveryClient
	onErr         func()
	DeliverClient peer.DeliverClient
}

func (c *StatefulClient) Deliver(ctx context.Context, opts ...ggrpc.CallOption) (peer.Deliver_DeliverClient, error) {
	return c.DeliverClient.Deliver(ctx, opts...)
}

func (c *StatefulClient) DeliverFiltered(ctx context.Context, opts ...ggrpc.CallOption) (peer.Deliver_DeliverFilteredClient, error) {
	return c.DeliverClient.DeliverFiltered(ctx, opts...)
}

func (c *StatefulClient) DeliverWithPrivateData(ctx context.Context, opts ...ggrpc.CallOption) (peer.Deliver_DeliverWithPrivateDataClient, error) {
	return c.DeliverClient.DeliverWithPrivateData(ctx, opts...)
}

func (c *StatefulClient) ProcessProposal(ctx context.Context, in *peer.SignedProposal, opts ...ggrpc.CallOption) (*peer.ProposalResponse, error) {
	res, err := c.EndorserClient.ProcessProposal(ctx, in, opts...)
	if err != nil {
		c.onErr()
	}
	return res, err
}

func (c *StatefulClient) Discover(ctx context.Context, in *discovery.SignedRequest, opts ...ggrpc.CallOption) (*discovery.Response, error) {
	res, err := c.DiscoveryClient.Discover(ctx, in, opts...)
	if err != nil {
		c.onErr()
	}
	return res, err
}

func (c *StatefulClient) Send(ctx context.Context, req *dclient.Request, auth *discovery.AuthInfo) (dclient.Response, error) {
	res, err := c.DC.Send(ctx, req, auth)
	if err != nil {
		c.onErr()
	}
	return res, err
}

type ClientWrapper struct {
	lock sync.RWMutex
	Client
	connect func() (*ggrpc.ClientConn, error)
	conn    *ggrpc.ClientConn
	signer  dclient.Signer
	address string
}

func (c *ClientWrapper) getOrConn() (*ggrpc.ClientConn, error) {
	c.lock.RLock()
	existingConn := c.conn
	c.lock.RUnlock()

	if existingConn != nil {
		return existingConn, nil
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	if c.conn != nil {
		return c.conn, nil
	}

	conn, err := c.connect()
	if err != nil {
		return nil, err
	}

	c.conn = conn

	return conn, nil
}

func (c *ClientWrapper) resetConn() {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.conn != nil {
		c.conn.Close()
	}
	c.conn = nil
}

func (c *ClientWrapper) Address() string {
	return c.address
}

func (c *ClientWrapper) Connection() (*ggrpc.ClientConn, error) {
	conn, err := c.getOrConn()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get connection to peer")
	}

	return conn, nil
}

func (c *ClientWrapper) EndorserClient() (peer.EndorserClient, error) {
	conn, err := c.getOrConn()
	if err != nil {
		return nil, errors.Wrap(err, "error getting connection to endorser")
	}
	return &StatefulClient{
		EndorserClient: peer.NewEndorserClient(conn),
		onErr:          c.resetConn,
	}, nil
}

func (c *ClientWrapper) DeliverClient() (peer.DeliverClient, error) {
	conn, err := c.getOrConn()
	if err != nil {
		return nil, errors.Wrap(err, "error getting connection to endorser")
	}
	return &StatefulClient{
		DeliverClient: peer.NewDeliverClient(conn),
		onErr:         c.resetConn,
	}, nil
}

func (c *ClientWrapper) DiscoveryClient() (DiscoveryClient, error) {
	dc := dclient.NewClient(
		func() (*ggrpc.ClientConn, error) {
			conn, err := c.getOrConn()
			if err != nil {
				return nil, errors.Wrap(err, "error getting connection to endorser")
			}
			return conn, nil
		},
		c.signer,
		1,
	)

	return &StatefulClient{
		DC:    dc,
		onErr: c.resetConn,
	}, nil
}

func (c *ClientWrapper) Close() {
	// Don't do anything
}

type CachingClientFactory struct {
	ClientFactory
	lock   sync.RWMutex
	Cache  map[string]Client
	Signer driver.Signer
}

func (cep *CachingClientFactory) NewClient(cc grpc.ConnectionConfig) (Client, error) {
	return cep.getOrCreateClient(cc.Address, func() (Client, error) {
		return cep.ClientFactory.NewClient(cc)
	})
}

func (cep *CachingClientFactory) getOrCreateClient(key string, newClient func() (Client, error)) (Client, error) {
	if cl, found := cep.lookup(key); found {
		return cl, nil
	}

	cep.lock.Lock()
	defer cep.lock.Unlock()

	if cl, found := cep.lookupNoLock(key); found {
		return cl, nil
	}

	cl, err := newClient()
	if err != nil {
		return nil, err
	}

	pc := cl.(*GRPCClient)

	cl = &ClientWrapper{
		connect: func() (*ggrpc.ClientConn, error) {
			return pc.NewConnection(pc.Address(), grpc.ServerNameOverride(pc.Sn))
		},
		address: pc.Address(),
		Client:  cl,
		signer:  cep.Signer.Sign,
	}

	logger.Debugf("Created new GRPCClient for [%s]", key)
	cep.Cache[key] = cl

	return cl, nil
}

func (cep *CachingClientFactory) lookupNoLock(key string) (Client, bool) {
	cl, ok := cep.Cache[key]
	return cl, ok
}

func (cep *CachingClientFactory) lookup(key string) (Client, bool) {
	cep.lock.RLock()
	defer cep.lock.RUnlock()

	cl, ok := cep.Cache[key]
	return cl, ok
}

type GRPCClientFactory struct {
	ConfigService driver.ConfigService
	Singer        driver.Signer
}

func (c *GRPCClientFactory) NewClient(cc grpc.ConnectionConfig) (Client, error) {
	logger.Debugf("Creating new peer GRPCClient for address [%s]", cc.Address)

	secOpts, err := grpc.CreateSecOpts(cc, grpc.TLSClientConfig{
		TLSClientAuthRequired: c.ConfigService.TLSClientAuthRequired(),
		TLSClientKeyFile:      c.ConfigService.TLSClientKeyFile(),
		TLSClientCertFile:     c.ConfigService.TLSClientCertFile(),
	})
	if err != nil {
		return nil, err
	}

	timeout := c.ConfigService.ClientConnTimeout()
	if timeout <= 0 {
		timeout = grpc.DefaultConnectionTimeout
	}
	clientConfig := grpc.ClientConfig{
		SecOpts: *secOpts,
		KaOpts: grpc.KeepaliveOptions{
			ClientInterval: c.ConfigService.KeepAliveClientInterval(),
			ClientTimeout:  c.ConfigService.KeepAliveClientTimeout(),
		},
		Timeout: timeout,
	}

	override := cc.ServerNameOverride
	if len(override) == 0 {
		override = c.ConfigService.TLSServerHostOverride()
	}

	gClient, err := grpc.NewGRPCClient(clientConfig)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create Client from config")
	}
	pClient := &GRPCClient{
		Signer:      c.Singer.Sign,
		Client:      gClient,
		PeerAddress: cc.Address,
		Sn:          override,
	}
	return pClient, nil
}
