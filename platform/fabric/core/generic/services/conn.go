/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package services

import (
	"context"
	"crypto/tls"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	dclient "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/discovery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger/fabric-protos-go-apiv2/discovery"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	ggrpc "google.golang.org/grpc"
)

type StatefulClient struct {
	peer.EndorserClient
	discovery.DiscoveryClient
	DC            DiscoveryClient
	onErr         func() error
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
		_ = c.onErr()
	}
	return res, err
}

func (c *StatefulClient) Discover(ctx context.Context, in *discovery.SignedRequest, opts ...ggrpc.CallOption) (*discovery.Response, error) {
	res, err := c.DiscoveryClient.Discover(ctx, in, opts...)
	if err != nil {
		_ = c.onErr()
	}
	return res, err
}

func (c *StatefulClient) Send(ctx context.Context, req *dclient.Request, auth *discovery.AuthInfo) (dclient.Response, error) {
	res, err := c.DC.Send(ctx, req, auth)
	if err != nil {
		_ = c.onErr()
	}
	return res, err
}

type resettableClient interface {
	PeerClient
	OrdererClient
	Reset() error
}

type ClientWrapper struct {
	client resettableClient
}

func NewClientWrapper(pc *GRPCClient) *ClientWrapper {
	return &ClientWrapper{client: NewLazyGRPCClient(pc)}
}

func (c *ClientWrapper) EndorserClient() (peer.EndorserClient, error) {
	cl, err := c.client.EndorserClient()
	if err != nil {
		return nil, err
	}
	return &StatefulClient{EndorserClient: cl, onErr: c.client.Reset}, nil
}

func (c *ClientWrapper) DeliverClient() (peer.DeliverClient, error) {
	cl, err := c.client.DeliverClient()
	if err != nil {
		return nil, err
	}
	return &StatefulClient{DeliverClient: cl, onErr: c.client.Reset}, nil
}

func (c *ClientWrapper) DiscoveryClient() (DiscoveryClient, error) {
	dc, err := c.client.DiscoveryClient()
	if err != nil {
		return nil, err
	}
	return &StatefulClient{DC: dc, onErr: c.client.Reset}, nil
}

func (c *ClientWrapper) OrdererClient() (ab.AtomicBroadcastClient, error) {
	return c.client.OrdererClient()
}

func (c *ClientWrapper) Certificate() tls.Certificate {
	return c.client.Certificate()
}

func (c *ClientWrapper) Address() string {
	return c.client.Address()
}

func (c *ClientWrapper) Close() {
	// Don't do anything
}

func NewCachingClientFactory(configService driver.ConfigService, signer driver.Signer) *CachingClientFactory {
	f := newFactory(configService, signer)

	return &CachingClientFactory{
		cachePeer: lazy.NewProviderWithKeyMapper(
			func(cc grpc.ConnectionConfig) string { return cc.Address },
			f.NewPeerClient,
		),
		cacheOrderer: lazy.NewProviderWithKeyMapper(
			func(cc grpc.ConnectionConfig) string { return cc.Address },
			f.NewOrdererClient,
		),
	}
}

type CachingClientFactory struct {
	cachePeer    lazy.Provider[grpc.ConnectionConfig, PeerClient]
	cacheOrderer lazy.Provider[grpc.ConnectionConfig, OrdererClient]
}

func (cep *CachingClientFactory) NewPeerClient(cc grpc.ConnectionConfig) (PeerClient, error) {
	return cep.cachePeer.Get(cc)
}

func (cep *CachingClientFactory) NewOrdererClient(cc grpc.ConnectionConfig) (OrdererClient, error) {
	return cep.cacheOrderer.Get(cc)
}

type GRPCClientFactory struct {
	ConfigService driver.ConfigService
	Signer        driver.Signer
}

func newFactory(configService driver.ConfigService, signer driver.Signer) *GRPCClientFactory {
	return &GRPCClientFactory{
		ConfigService: configService,
		Signer:        signer,
	}
}

func (c *GRPCClientFactory) NewPeerClient(cc grpc.ConnectionConfig) (PeerClient, error) {
	cl, err := c.newClient(cc)
	if err != nil {
		return nil, err
	}

	return NewClientWrapper(cl), nil
}

func (c *GRPCClientFactory) NewOrdererClient(cc grpc.ConnectionConfig) (OrdererClient, error) {
	cl, err := c.newClient(cc)
	if err != nil {
		return nil, err
	}

	return NewClientWrapper(cl), nil
}

func (c *GRPCClientFactory) newClient(cc grpc.ConnectionConfig) (*GRPCClient, error) {
	logger.Debugf("Creating new peer GRPCClient for address [%s]", cc.Address)

	secOpts, err := grpc.CreateSecOpts(cc, grpc.TLSClientConfig{
		TLSClientAuthRequired: cc.TLSClientSideAuth,
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
	return NewGRPCClient(gClient, cc.Address, override, c.Signer.Sign), nil
}
