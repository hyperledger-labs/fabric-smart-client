/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package services

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/discovery"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	ggrpc "google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	dclient "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/discovery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type fakeEndorserClient struct {
	res *peer.ProposalResponse
	err error
}

func (f *fakeEndorserClient) ProcessProposal(context.Context, *peer.SignedProposal, ...ggrpc.CallOption) (*peer.ProposalResponse, error) {
	return f.res, f.err
}

type fakeDiscoveryClient struct {
	res *discovery.Response
	err error
}

func (f *fakeDiscoveryClient) Discover(context.Context, *discovery.SignedRequest, ...ggrpc.CallOption) (*discovery.Response, error) {
	return f.res, f.err
}

type fakeDC struct {
	res dclient.Response
	err error
}

func (f *fakeDC) Send(context.Context, *dclient.Request, *discovery.AuthInfo) (dclient.Response, error) {
	return f.res, f.err
}

type fakeResponse struct {
	dclient.Response
}

type fakeDeliverStream struct {
	ggrpc.BidiStreamingClient[common.Envelope, peer.DeliverResponse]
}

// fakeDeliverClient records the arguments of the last call and returns stream and err.
type fakeDeliverClient struct {
	stream peer.Deliver_DeliverClient
	err    error
	ctx    context.Context
	opts   []ggrpc.CallOption
}

func (f *fakeDeliverClient) record(ctx context.Context, opts []ggrpc.CallOption) (peer.Deliver_DeliverClient, error) {
	f.ctx, f.opts = ctx, opts
	return f.stream, f.err
}

func (f *fakeDeliverClient) Deliver(ctx context.Context, opts ...ggrpc.CallOption) (peer.Deliver_DeliverClient, error) {
	return f.record(ctx, opts)
}

func (f *fakeDeliverClient) DeliverFiltered(ctx context.Context, opts ...ggrpc.CallOption) (peer.Deliver_DeliverFilteredClient, error) {
	return f.record(ctx, opts)
}

func (f *fakeDeliverClient) DeliverWithPrivateData(ctx context.Context, opts ...ggrpc.CallOption) (peer.Deliver_DeliverWithPrivateDataClient, error) {
	return f.record(ctx, opts)
}

type fakeBroadcastClient struct {
	ab.AtomicBroadcastClient
}

type fakeResettableClient struct {
	endorser  peer.EndorserClient
	deliver   peer.DeliverClient
	discovery DiscoveryClient
	orderer   ab.AtomicBroadcastClient
	err       error
	cert      tls.Certificate
	address   string
	resets    int
	closes    int
}

func (f *fakeResettableClient) EndorserClient() (peer.EndorserClient, error) {
	return f.endorser, f.err
}

func (f *fakeResettableClient) DeliverClient() (peer.DeliverClient, error) { return f.deliver, f.err }

func (f *fakeResettableClient) DiscoveryClient() (DiscoveryClient, error) {
	return f.discovery, f.err
}

func (f *fakeResettableClient) OrdererClient() (ab.AtomicBroadcastClient, error) {
	return f.orderer, f.err
}

func (f *fakeResettableClient) Certificate() tls.Certificate { return f.cert }

func (f *fakeResettableClient) Address() string { return f.address }

func (f *fakeResettableClient) Close() { f.closes++ }

func (f *fakeResettableClient) Reset() error {
	f.resets++
	return nil
}

type fakeConfigService struct {
	driver.ConfigService
	timeout time.Duration
}

func (f *fakeConfigService) ClientConnTimeout() time.Duration { return f.timeout }

func (*fakeConfigService) ClientKeepAliveConfig() *grpc.ClientKeepAliveConfig { return nil }

type fakeSigner struct{}

func (fakeSigner) Sign(msg []byte) ([]byte, error) { return msg, nil }

func TestStatefulClientDeliver(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "value")
	opt := ggrpc.WaitForReady(true)
	errDeliver := errors.New("deliver failed")

	for name, call := range map[string]func(*StatefulClient) (peer.Deliver_DeliverClient, error){
		"Deliver":                func(c *StatefulClient) (peer.Deliver_DeliverClient, error) { return c.Deliver(ctx, opt) },
		"DeliverFiltered":        func(c *StatefulClient) (peer.Deliver_DeliverClient, error) { return c.DeliverFiltered(ctx, opt) },
		"DeliverWithPrivateData": func(c *StatefulClient) (peer.Deliver_DeliverClient, error) { return c.DeliverWithPrivateData(ctx, opt) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for caseName, callErr := range map[string]error{"success": nil, "failure": errDeliver} {
				t.Run(caseName, func(t *testing.T) {
					t.Parallel()
					fake := &fakeDeliverClient{stream: &fakeDeliverStream{}, err: callErr}
					resets := 0
					c := &StatefulClient{DeliverClient: fake, onErr: func() error { resets++; return nil }}

					stream, err := call(c)
					require.Equal(t, callErr, err)
					require.Same(t, fake.stream, stream)
					require.Equal(t, ctx, fake.ctx)
					require.Equal(t, []ggrpc.CallOption{opt}, fake.opts)
					require.Zero(t, resets, "deliver calls do not reset the connection")
				})
			}
		})
	}
}

func TestStatefulClientResetsOnError(t *testing.T) {
	t.Parallel()

	endorserRes := &peer.ProposalResponse{}
	discoverRes := &discovery.Response{}
	var sendRes dclient.Response = &fakeResponse{}
	errRPC := errors.New("rpc failed")

	for name, call := range map[string]func(*StatefulClient) (any, any, error){
		"ProcessProposal": func(c *StatefulClient) (any, any, error) {
			res, err := c.ProcessProposal(context.Background(), &peer.SignedProposal{})
			return endorserRes, res, err
		},
		"Discover": func(c *StatefulClient) (any, any, error) {
			res, err := c.Discover(context.Background(), &discovery.SignedRequest{})
			return discoverRes, res, err
		},
		"Send": func(c *StatefulClient) (any, any, error) {
			res, err := c.Send(context.Background(), &dclient.Request{}, &discovery.AuthInfo{})
			return sendRes, res, err
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range []struct {
				name   string
				err    error
				resets int
			}{
				{name: "success", err: nil, resets: 0},
				{name: "failure", err: errRPC, resets: 1},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					resets := 0
					c := &StatefulClient{
						EndorserClient:  &fakeEndorserClient{res: endorserRes, err: tc.err},
						DiscoveryClient: &fakeDiscoveryClient{res: discoverRes, err: tc.err},
						DC:              &fakeDC{res: sendRes, err: tc.err},
						// A failing reset must not replace the error of the call.
						onErr: func() error { resets++; return errors.New("reset failed") },
					}

					want, got, err := call(c)
					require.Equal(t, tc.err, err)
					require.Same(t, want, got)
					require.Equal(t, tc.resets, resets)
				})
			}
		})
	}
}

func TestClientWrapper(t *testing.T) {
	t.Parallel()

	errRPC := errors.New("rpc failed")

	t.Run("clients are wrapped in a StatefulClient bound to Reset", func(t *testing.T) {
		t.Parallel()
		fake := &fakeResettableClient{
			endorser:  &fakeEndorserClient{err: errRPC},
			deliver:   &fakeDeliverClient{},
			discovery: &fakeDC{err: errRPC},
		}
		w := &ClientWrapper{client: fake}

		ec, err := w.EndorserClient()
		require.NoError(t, err)
		require.IsType(t, &StatefulClient{}, ec)
		_, err = ec.ProcessProposal(context.Background(), &peer.SignedProposal{})
		require.ErrorIs(t, err, errRPC)
		require.Equal(t, 1, fake.resets)

		dc, err := w.DiscoveryClient()
		require.NoError(t, err)
		require.IsType(t, &StatefulClient{}, dc)
		_, err = dc.Send(context.Background(), &dclient.Request{}, &discovery.AuthInfo{})
		require.ErrorIs(t, err, errRPC)
		require.Equal(t, 2, fake.resets)

		del, err := w.DeliverClient()
		require.NoError(t, err)
		require.IsType(t, &StatefulClient{}, del)
		require.Same(t, fake.deliver, del.(*StatefulClient).DeliverClient)
	})

	t.Run("errors propagate", func(t *testing.T) {
		t.Parallel()
		w := &ClientWrapper{client: &fakeResettableClient{err: errRPC}}

		ec, err := w.EndorserClient()
		require.ErrorIs(t, err, errRPC)
		require.Nil(t, ec)
		dc, err := w.DeliverClient()
		require.ErrorIs(t, err, errRPC)
		require.Nil(t, dc)
		disc, err := w.DiscoveryClient()
		require.ErrorIs(t, err, errRPC)
		require.Nil(t, disc)
		oc, err := w.OrdererClient()
		require.ErrorIs(t, err, errRPC)
		require.Nil(t, oc)
	})

	t.Run("delegates", func(t *testing.T) {
		t.Parallel()
		fake := &fakeResettableClient{
			orderer: &fakeBroadcastClient{},
			cert:    tls.Certificate{Certificate: [][]byte{[]byte("cert")}},
			address: "peer0:7051",
		}
		w := &ClientWrapper{client: fake}

		oc, err := w.OrdererClient()
		require.NoError(t, err)
		require.Same(t, fake.orderer, oc)
		require.Equal(t, fake.cert, w.Certificate())
		require.Equal(t, "peer0:7051", w.Address())
	})

	t.Run("Close does not touch the underlying client", func(t *testing.T) {
		t.Parallel()
		fake := &fakeResettableClient{}
		w := &ClientWrapper{client: fake}

		w.Close()
		require.Zero(t, fake.closes)
		require.Zero(t, fake.resets)
	})
}

func TestCachingClientFactory(t *testing.T) {
	t.Parallel()

	f := NewCachingClientFactory(&fakeConfigService{}, fakeSigner{})

	p1, err := f.NewPeerClient(grpc.ConnectionConfig{Address: "peer0:7051"})
	require.NoError(t, err)
	p2, err := f.NewPeerClient(grpc.ConnectionConfig{Address: "peer0:7051", ConnectionTimeout: time.Minute})
	require.NoError(t, err)
	p3, err := f.NewPeerClient(grpc.ConnectionConfig{Address: "peer1:7051"})
	require.NoError(t, err)
	require.Same(t, p1, p2, "configs with the same address share a client")
	require.NotSame(t, p1, p3)
	require.Equal(t, "peer1:7051", p3.Address())

	o1, err := f.NewOrdererClient(grpc.ConnectionConfig{Address: "peer0:7051"})
	require.NoError(t, err)
	o2, err := f.NewOrdererClient(grpc.ConnectionConfig{Address: "peer0:7051"})
	require.NoError(t, err)
	o3, err := f.NewOrdererClient(grpc.ConnectionConfig{Address: "orderer:7050"})
	require.NoError(t, err)
	require.Same(t, o1, o2)
	require.NotSame(t, o1, o3)
	require.NotSame(t, p1, o1, "peer and orderer clients are cached separately")
}

func TestClientFactory(t *testing.T) {
	t.Parallel()

	f := NewClientFactory(&fakeConfigService{timeout: time.Second}, fakeSigner{})

	pc, err := f.NewPeerClient(grpc.ConnectionConfig{Address: "peer0:7051"})
	require.NoError(t, err)
	require.Equal(t, "peer0:7051", pc.Address())
	oc, err := f.NewOrdererClient(grpc.ConnectionConfig{Address: "orderer:7050"})
	require.NoError(t, err)
	require.Equal(t, "orderer:7050", oc.Address())

	// Mutual TLS without a keypair cannot build a gRPC client.
	badTLS := grpc.ConnectionConfig{Address: "peer1:7051", TLS: grpc.SecureOptions{UseTLS: true, RequireClientCert: true}}
	_, err = f.NewPeerClient(badTLS)
	require.ErrorContains(t, err, "failed to create Client from config")
	_, err = f.NewOrdererClient(badTLS)
	require.ErrorContains(t, err, "failed to create Client from config")
}
