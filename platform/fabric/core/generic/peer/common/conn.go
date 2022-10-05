/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger/fabric-protos-go/discovery"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	discovery2 "github.com/hyperledger/fabric/discovery/client"
	"github.com/pkg/errors"
	grpc2 "google.golang.org/grpc"
)

var logger = flogging.MustGetLogger("fabric-sdk.core.generic.peer.conn")

type ConnCreator interface {
	NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer.Client, error)
}

type statefulClient struct {
	pb.EndorserClient
	discovery.DiscoveryClient
	DC            peer.DiscoveryClient
	onErr         func()
	DeliverClient pb.DeliverClient
}

func (sc *statefulClient) Deliver(ctx context.Context, opts ...grpc2.CallOption) (pb.Deliver_DeliverClient, error) {
	return sc.DeliverClient.Deliver(ctx, opts...)
}

func (sc *statefulClient) DeliverFiltered(ctx context.Context, opts ...grpc2.CallOption) (pb.Deliver_DeliverFilteredClient, error) {
	return sc.DeliverClient.DeliverFiltered(ctx, opts...)
}

func (sc *statefulClient) DeliverWithPrivateData(ctx context.Context, opts ...grpc2.CallOption) (pb.Deliver_DeliverWithPrivateDataClient, error) {
	return sc.DeliverClient.DeliverWithPrivateData(ctx, opts...)
}

func (sc *statefulClient) ProcessProposal(ctx context.Context, in *pb.SignedProposal, opts ...grpc2.CallOption) (*pb.ProposalResponse, error) {
	res, err := sc.EndorserClient.ProcessProposal(ctx, in, opts...)
	if err != nil {
		sc.onErr()
	}
	return res, err
}

func (sc *statefulClient) Discover(ctx context.Context, in *discovery.SignedRequest, opts ...grpc2.CallOption) (*discovery.Response, error) {
	res, err := sc.DiscoveryClient.Discover(ctx, in, opts...)
	if err != nil {
		sc.onErr()
	}
	return res, err
}

func (sc *statefulClient) Send(ctx context.Context, req *discovery2.Request, auth *discovery.AuthInfo) (discovery2.Response, error) {
	res, err := sc.DC.Send(ctx, req, auth)
	if err != nil {
		sc.onErr()
	}
	return res, err
}

type peerClient struct {
	lock sync.RWMutex
	peer.Client
	connect func() (*grpc2.ClientConn, error)
	conn    *grpc2.ClientConn
	signer  discovery2.Signer
	address string
}

func (pc *peerClient) getOrConn() (*grpc2.ClientConn, error) {
	pc.lock.RLock()
	existingConn := pc.conn
	pc.lock.RUnlock()

	if existingConn != nil {
		return existingConn, nil
	}

	pc.lock.Lock()
	defer pc.lock.Unlock()

	if pc.conn != nil {
		return pc.conn, nil
	}

	conn, err := pc.connect()
	if err != nil {
		return nil, err
	}

	pc.conn = conn

	return conn, nil
}

func (pc *peerClient) resetConn() {
	pc.lock.Lock()
	defer pc.lock.Unlock()

	if pc.conn != nil {
		pc.conn.Close()
	}
	pc.conn = nil
}

func (pc *peerClient) Address() string {
	return pc.address
}

func (pc *peerClient) Connection() (*grpc2.ClientConn, error) {
	conn, err := pc.getOrConn()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get connection to peer")
	}

	return conn, nil
}

func (pc *peerClient) Endorser() (pb.EndorserClient, error) {
	conn, err := pc.getOrConn()
	if err != nil {
		return nil, errors.Wrap(err, "error getting connection to endorser")
	}
	return &statefulClient{
		EndorserClient: pb.NewEndorserClient(conn),
		onErr:          pc.resetConn,
	}, nil
}

func (pc *peerClient) DeliverClient() (pb.DeliverClient, error) {
	conn, err := pc.getOrConn()
	if err != nil {
		return nil, errors.Wrap(err, "error getting connection to endorser")
	}
	return &statefulClient{
		DeliverClient: pb.NewDeliverClient(conn),
		onErr:         pc.resetConn,
	}, nil
}

func (pc *peerClient) DiscoveryClient() (peer.DiscoveryClient, error) {
	dc := discovery2.NewClient(
		func() (*grpc2.ClientConn, error) {
			conn, err := pc.getOrConn()
			if err != nil {
				return nil, errors.Wrap(err, "error getting connection to endorser")
			}
			return conn, nil
		},
		pc.signer,
		1,
	)

	return &statefulClient{
		DC:    dc,
		onErr: pc.resetConn,
	}, nil
}

func (pc *peerClient) Discovery() (discovery.DiscoveryClient, error) {
	conn, err := pc.getOrConn()
	if err != nil {
		return nil, errors.Wrap(err, "error getting connection to peer")
	}
	return &statefulClient{
		DiscoveryClient: discovery.NewDiscoveryClient(conn),
		onErr:           pc.resetConn,
	}, nil
}

func (pc *peerClient) Close() {
	// Don't do anything
}

type CachingEndorserPool struct {
	ConnCreator
	lock   sync.RWMutex
	Cache  map[string]peer.Client
	Signer discovery2.Signer
}

func (cep *CachingEndorserPool) NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer.Client, error) {
	return cep.getOrCreateClient(cc.Address, func() (peer.Client, error) {
		return cep.ConnCreator.NewPeerClientForAddress(cc)
	})
}

func (cep *CachingEndorserPool) getOrCreateClient(key string, newClient func() (peer.Client, error)) (peer.Client, error) {
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

	pc := cl.(*PeerClient)

	cl = &peerClient{
		connect: func() (*grpc2.ClientConn, error) {
			return pc.NewConnection(pc.Address(), grpc.ServerNameOverride(pc.Sn))
		},
		address: pc.Address(),
		Client:  cl,
		signer:  cep.Signer,
	}

	logger.Debugf("Created new client for [%s]", key)
	cep.Cache[key] = cl

	return cl, nil
}

func (cep *CachingEndorserPool) lookupNoLock(key string) (peer.Client, bool) {
	cl, ok := cep.Cache[key]
	return cl, ok
}

func (cep *CachingEndorserPool) lookup(key string) (peer.Client, bool) {
	cep.lock.RLock()
	defer cep.lock.RUnlock()

	cl, ok := cep.Cache[key]
	return cl, ok
}
