/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"context"
	"net"

	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	p2pproto "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/grpc/protos"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

type client struct {
	nodeID host2.PeerID
	client *grpc2.Client
}

func newClient(nodeID host2.PeerID, clientConfig grpc2.ClientConfig) (*client, error) {
	grpcClient, err := grpc2.NewGRPCClient(clientConfig)
	if err != nil {
		return nil, err
	}
	return &client{
		nodeID: nodeID,
		client: grpcClient,
	}, nil
}

func (c *client) OpenStream(info host2.StreamInfo, ctx context.Context) (host2.P2PStream, error) {
	if len(info.RemotePeerAddress) == 0 {
		return nil, errors.New("remote peer address is empty")
	}

	tlsOptions := make([]grpc2.TLSOption, 0, 1)
	if host, _, err := net.SplitHostPort(info.RemotePeerAddress); err == nil && host != "" {
		tlsOptions = append(tlsOptions, grpc2.ServerNameOverride(host))
	}

	conn, err := c.client.NewConnection(info.RemotePeerAddress, tlsOptions...)
	if err != nil {
		return nil, err
	}

	stream, err := p2pproto.NewP2PServiceClient(conn).OpenStream(ctx)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	spanContext := trace.SpanContextFromContext(ctx)
	marshalledSpanContext, err := tracing.MarshalContext(spanContext)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := stream.Send(&p2pproto.StreamPacket{
		Body: &p2pproto.StreamPacket_Open{
			Open: &p2pproto.StreamOpen{
				ContextId:   info.ContextID,
				SessionId:   info.SessionID,
				PeerId:      string(c.nodeID),
				SpanContext: marshalledSpanContext,
			},
		},
	}); err != nil {
		_ = conn.Close()
		return nil, errors.Wrapf(err, "failed to open grpc p2p stream")
	}

	return NewGRPCStream(stream, conn, ctx, info), nil
}

func (c *client) Close() error {
	if c.client != nil {
		c.client.Close()
	}
	return nil
}
