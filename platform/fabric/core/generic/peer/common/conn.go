/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"sync"

	"github.com/hyperledger/fabric-protos-go/discovery"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	grpc2 "google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ConnCreator interface {
	NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer.Client, error)

	// NewPeerClientForIdentity creates an instance of a PeerClient using the
	// provided peer identity
	NewPeerClientForIdentity(peer view.Identity) (peer.Client, error)
}

type statefulClient struct {
	pb.EndorserClient
	discovery.DiscoveryClient
	onErr func()
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

type peerClient struct {
	lock sync.RWMutex
	peer.Client
	connect func() (*grpc2.ClientConn, error)
	conn    *grpc2.ClientConn
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

func (pc *peerClient) Endorser() (pb.EndorserClient, error) {
	return &statefulClient{
		EndorserClient: pb.NewEndorserClient(pc.conn),
		onErr:          pc.resetConn,
	}, nil
}

func (pc *peerClient) Discovery() (discovery.DiscoveryClient, error) {
	return &statefulClient{
		DiscoveryClient: discovery.NewDiscoveryClient(pc.conn),
		onErr:           pc.resetConn,
	}, nil
}

func (pc *peerClient) Close() {
	// Don't do anything
}

type CachingEndorserPool struct {
	ConnCreator
	lock  sync.RWMutex
	Cache map[string]peer.Client
}

func (cep *CachingEndorserPool) NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer.Client, error) {
	return cep.getOrCreateClient(cc.Address, func() (peer.Client, error) {
		return cep.ConnCreator.NewPeerClientForAddress(cc)
	})
}

func (cep *CachingEndorserPool) NewPeerClientForIdentity(p view.Identity) (peer.Client, error) {
	return cep.getOrCreateClient(string(p), func() (peer.Client, error) {
		return cep.ConnCreator.NewPeerClientForIdentity(p)
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
			return pc.NewConnection(pc.Address, grpc.ServerNameOverride(pc.Sn))
		},
		Client: cl,
	}

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
