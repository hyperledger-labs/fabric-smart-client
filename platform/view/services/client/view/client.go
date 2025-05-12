/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"io"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var logger = logging.MustGetLogger()

type TimeFunc func() time.Time

type SigningIdentity interface {
	Serialize() ([]byte, error)

	Sign(msg []byte) ([]byte, error)
}

//go:generate counterfeiter -o mock/view_peer_client.go -fake-name ViewServiceClient . ViewServiceClient

// ViewServiceClient defines an interface that creates a client to communicate with the view service in a peer
type ViewServiceClient interface {
	// CreateViewClient creates a grpc connection and client to view peer
	CreateViewClient() (*grpc.ClientConn, protos.ViewServiceClient, error)

	// Certificate returns tls client certificate
	Certificate() *tls.Certificate
}

// ViewServiceClientImpl implements ViewServiceClient interface
type ViewServiceClientImpl struct {
	Address            string
	ServerNameOverride string
	GRPCClient         *grpc2.Client
}

func (pc *ViewServiceClientImpl) CreateViewClient() (*grpc.ClientConn, protos.ViewServiceClient, error) {
	logger.Debugf("opening connection to [%s]", pc.Address)
	conn, err := pc.GRPCClient.NewConnection(pc.Address)
	if err != nil {
		logger.Errorf("failed creating connection to [%s]: [%s]", pc.Address, err)
		return conn, nil, errors.Wrapf(err, "failed creating connection to [%s]", pc.Address)
	}
	logger.Debugf("opening connection to [%s], done.", pc.Address)

	return conn, protos.NewViewServiceClient(conn), nil
}

func (pc *ViewServiceClientImpl) Certificate() *tls.Certificate {
	cert := pc.GRPCClient.Certificate()
	return &cert
}

// client implements network.Client interface
type client struct {
	Address           string
	ViewServiceClient ViewServiceClient
	RandomnessReader  io.Reader
	Time              TimeFunc
	SigningIdentity   SigningIdentity
	hasher            hash.Hasher
	tracer            trace.Tracer
}

func NewClient(config *Config, sID SigningIdentity, hasher hash.Hasher, tracerProvider trace.TracerProvider) (*client, error) {
	// create a grpc client for view peer
	grpcClient, err := grpc2.CreateGRPCClient(config.ConnectionConfig)
	if err != nil {
		return nil, err
	}

	return &client{
		Address:          config.ConnectionConfig.Address,
		RandomnessReader: rand.Reader,
		Time:             time.Now,
		ViewServiceClient: &ViewServiceClientImpl{
			Address:            config.ConnectionConfig.Address,
			ServerNameOverride: config.ConnectionConfig.ServerNameOverride,
			GRPCClient:         grpcClient,
		},
		SigningIdentity: sID,
		hasher:          hasher,
		tracer: tracerProvider.Tracer("client", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace: "viewsdk",
		})),
	}, nil
}

func (s *client) CallView(fid string, input []byte) (interface{}, error) {
	return s.CallViewWithContext(context.Background(), fid, input)
}

func (s *client) CallViewWithContext(ctx context.Context, fid string, input []byte) (interface{}, error) {
	logger.Debugf("Calling view [%s] on input [%s]", fid, string(input))
	payload := &protos.Command_CallView{CallView: &protos.CallView{
		Fid:   fid,
		Input: input,
	}}
	sc, err := s.CreateSignedCommand(payload, s.SigningIdentity)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating signed command for [%s,%s]", fid, string(input))
	}

	ctx, span := s.tracer.Start(ctx, "GrpcViewInvocation", tracing.WithAttributes(tracing.String("fid", fid)), trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	commandResp, err := s.processCommand(ctx, sc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed process command for [%s,%s]", fid, string(input))
	}

	if commandResp.GetCallViewResponse() == nil {
		return nil, errors.New("expected initiate view response, got nothing")
	}
	return commandResp.GetCallViewResponse().GetResult(), nil
}

func (s *client) Initiate(fid string, in []byte) (string, error) {
	panic("implement me")
}

func (s *client) StreamCallView(fid string, input []byte) (*Stream, error) {
	logger.Debugf("Streaming view call [%s] on input [%s]", fid, string(input))
	payload := &protos.Command_CallView{CallView: &protos.CallView{
		Fid:   fid,
		Input: input,
	}}
	sc, err := s.CreateSignedCommand(payload, s.SigningIdentity)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating signed command for [%s,%s]", fid, string(input))
	}

	conn, scc, err := s.streamCommand(context.Background(), sc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed stream command for [%s,%s]", fid, string(input))
	}

	return &Stream{conn: conn, scc: scc}, nil
}

// processCommand calls view client to send grpc request and returns a CommandResponse
func (s *client) processCommand(ctx context.Context, sc *protos.SignedCommand) (*protos.CommandResponse, error) {
	logger.Debugf("get view service client...")
	conn, client, err := s.ViewServiceClient.CreateViewClient()
	logger.Debugf("get view service client...done")
	if conn != nil {
		logger.Debugf("get view service client...got a connection")
		defer utils.IgnoreErrorFunc(conn.Close)
	}
	if err != nil {
		logger.Errorf("failed creating view client [%s]", err)
		return nil, errors.Wrap(err, "failed creating view client")
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("process command [%s]", sc.String())
	}
	scr, err := client.ProcessCommand(ctx, sc)
	if err != nil {
		logger.Errorf("failed view client process command [%s]", err)
		return nil, errors.Wrap(err, "failed view client process command")
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("parse answer [%s]", hash.Hashable(scr.Response).String())
	}
	commandResp := &protos.CommandResponse{}
	err = proto.Unmarshal(scr.Response, commandResp)
	if err != nil {
		logger.Errorf("failed to unmarshal command response [%s]", err)
		return nil, errors.Wrapf(err, "failed to unmarshal command response")
	}
	if commandResp.GetErr() != nil {
		logger.Errorf("error from view during process command: %s", commandResp.GetErr().GetMessage())
		return nil, errors.Errorf("error from view during process command: %s", commandResp.GetErr().GetMessage())
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("process command [%s] done", sc.String())
	}
	return commandResp, nil
}

// streamCommand calls view client to send grpc request and returns a CommandResponse
func (s *client) streamCommand(ctx context.Context, sc *protos.SignedCommand) (*grpc.ClientConn, protos.ViewService_StreamCommandClient, error) {
	logger.Debugf("get view service client...")
	conn, client, err := s.ViewServiceClient.CreateViewClient()
	logger.Debugf("get view service client...done")
	if conn != nil {
		logger.Debugf("get view service client...got a connection")
	}
	if err != nil {
		logger.Errorf("failed creating view client [%s]", err)
		return nil, nil, errors.Wrap(err, "failed creating view client")
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("stream command [%s]", sc.String())
	}
	streamCommandClient, err := client.StreamCommand(ctx)
	if err != nil {
		logger.Errorf("failed view client stream command [%s]", err)
		return nil, nil, errors.Wrap(err, "failed view client stream command")
	}
	if err := streamCommandClient.Send(sc); err != nil {
		return nil, nil, errors.Wrapf(err, "failed to send signed command")
	}

	return conn, streamCommandClient, nil
}

func (s *client) CreateSignedCommand(payload interface{}, signingIdentity SigningIdentity) (*protos.SignedCommand, error) {
	command, err := commandFromPayload(payload)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, 32)
	_, err = io.ReadFull(s.RandomnessReader, nonce)
	if err != nil {
		return nil, err
	}

	ts := timestamppb.New(s.Time())
	if err := ts.CheckValid(); err != nil {
		return nil, err
	}

	creator, err := signingIdentity.Serialize()
	if err != nil {
		return nil, err
	}

	// check for client certificate and compute SHA2-256 on certificate if present
	tlsCertHash, err := grpc2.GetTLSCertHash(s.ViewServiceClient.Certificate(), s.hasher)
	if err != nil {
		return nil, err
	}
	command.Header = &protos.Header{
		Timestamp:   ts,
		Nonce:       nonce,
		Creator:     creator,
		TlsCertHash: tlsCertHash,
	}

	raw, err := proto.Marshal(command)
	if err != nil {
		return nil, err
	}

	signature, err := signingIdentity.Sign(raw)
	if err != nil {
		return nil, err
	}

	sc := &protos.SignedCommand{
		Command:   raw,
		Signature: signature,
	}
	return sc, nil
}

func commandFromPayload(payload interface{}) (*protos.Command, error) {
	switch t := payload.(type) {
	case *protos.Command_InitiateView:
		return &protos.Command{Payload: t}, nil
	case *protos.Command_CallView:
		return &protos.Command{Payload: t}, nil
	default:
		return nil, errors.Errorf("command type not recognized: %T", t)
	}
}
