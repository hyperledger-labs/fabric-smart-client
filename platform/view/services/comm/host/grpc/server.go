/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"context"
	"net"
	"time"

	"go.opentelemetry.io/otel/trace"
	gogrpc "google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	p2pproto "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/grpc/protos"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

type server struct {
	grpcServer *grpc2.GRPCServer
}

func newServer(address string, config grpc2.ServerConfig) (*server, error) {
	grpcServer, err := grpc2.NewGRPCServer(address, config)
	if err != nil {
		return nil, err
	}
	return &server{grpcServer: grpcServer}, nil
}

func (s *server) Addr() string {
	return s.grpcServer.Address()
}

func (s *server) Start(newStreamCallback func(stream host2.P2PStream)) error {
	p2pproto.RegisterP2PServiceServer(s.grpcServer.Server(), &service{newStreamCallback: newStreamCallback})
	return s.grpcServer.Start()
}

func (s *server) Close() error {
	s.grpcServer.Stop()
	return nil
}

type service struct {
	p2pproto.UnimplementedP2PServiceServer
	newStreamCallback func(stream host2.P2PStream)
}

func (s *service) OpenStream(stream gogrpc.BidiStreamingServer[p2pproto.StreamPacket, p2pproto.StreamPacket]) error {
	expectedPeerID, err := expectedPeerID(stream.Context())
	if err != nil {
		return errors.Wrapf(err, "failed extracting expected peerID from TLS certificate")
	}

	packet, err := stream.Recv()
	if err != nil {
		return errors.Wrapf(err, "failed to read stream open frame")
	}
	openBody, ok := packet.Body.(*p2pproto.StreamPacket_Open)
	if !ok || openBody.Open == nil {
		return errors.New("missing stream open frame")
	}
	if openBody.Open.PeerId != expectedPeerID {
		return errors.Errorf("peer identity binding failed: claimed [%s], tls [%s]", openBody.Open.PeerId, expectedPeerID)
	}

	spanContext, err := tracing.UnmarshalContext(openBody.Open.SpanContext)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal span context")
	}

	info := host2.StreamInfo{
		RemotePeerID:      openBody.Open.PeerId,
		RemotePeerAddress: remoteAddress(stream.Context()),
		ContextID:         openBody.Open.ContextId,
		SessionID:         openBody.Open.SessionId,
	}
	p2pStream := NewGRPCStream(stream, nil, trace.ContextWithRemoteSpanContext(context.TODO(), spanContext), info)
	s.newStreamCallback(p2pStream)
	<-p2pStream.Context().Done()
	return nil
}

func waitForServerReady(address string) error {
	target := address
	host, port, err := net.SplitHostPort(address)
	if err == nil && (host == "" || host == "0.0.0.0" || host == "::") {
		target = net.JoinHostPort("127.0.0.1", port)
	}
	conn, err := net.DialTimeout("tcp", target, 100*time.Millisecond)
	if err != nil {
		return err
	}
	return conn.Close()
}
