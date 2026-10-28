/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"time"

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
)

var logger = logging.MustGetLogger()

type TimeFunc func() time.Time

// SigningIdentity models an identity that can sign messages.
type SigningIdentity interface {
	Serialize() ([]byte, error)

	Sign(msg []byte) ([]byte, error)
}

// Client calls views on a node through its view service. It signs every command with
// SigningIdentity and accepts a response only if it answers the command sent.
type Client struct {
	Transport        Transport
	RandomnessReader io.Reader
	Time             TimeFunc
	SigningIdentity  SigningIdentity
	tracer           trace.Tracer
}

// NewClient returns a view client for the node described by config. config.Transport
// selects how the client reaches the node (see [TransportType]; empty means TransportGRPC).
// It returns an error if config or config.ConnectionConfig is nil, if the transport is
// unknown, or if the transport cannot be configured.
func NewClient(config *Config, signer SigningIdentity, tracerProvider tracing.Provider) (*Client, error) {
	if config == nil {
		return nil, errors.New("missing client config")
	}
	if config.ConnectionConfig == nil {
		return nil, errors.New("missing fsc peer connection config")
	}
	var transport Transport
	switch config.Transport {
	case "", TransportGRPC:
		grpcClient, err := grpc.CreateGRPCClient(config.ConnectionConfig)
		if err != nil {
			return nil, err
		}
		transport = &grpcTransport{address: config.ConnectionConfig.Address, client: grpcClient}
	case TransportHTTP:
		t, err := newHTTPTransport(config.ConnectionConfig)
		if err != nil {
			return nil, err
		}
		transport = t
	default:
		return nil, errors.Errorf("unknown view client transport [%s]", config.Transport)
	}
	return &Client{
		Transport:        transport,
		RandomnessReader: rand.Reader,
		Time:             time.Now,
		SigningIdentity:  signer,
		tracer:           tracerProvider.Tracer("client", tracing.WithMetricsOpts(tracing.MetricsOpts{})),
	}, nil
}

// CallView calls the given view with the given input.
//
// The client sends the call once and never retries it, because views are not
// idempotent. If the client cannot connect to the node, the call fails before
// the request is sent and the view does not run. If the call fails after the
// request is sent, for example when the connection drops or a deadline expires,
// the view may have run fully or partially, and the caller decides whether
// running it again is safe.
func (s *Client) CallView(fid string, input []byte) (any, error) {
	return s.CallViewWithContext(context.Background(), fid, input)
}

// CallViewWithContext calls the given view with the given input and go context.
//
// The client sends the call once and never retries it, because views are not
// idempotent. If the client cannot connect to the node, the call fails before
// the request is sent and the view does not run. If the call fails after the
// request is sent, for example when the connection drops or a deadline expires,
// the view may have run fully or partially, and the caller decides whether
// running it again is safe.
func (s *Client) CallViewWithContext(ctx context.Context, fid string, input []byte) (any, error) {
	logger.Debugf("Calling view [%s] on input [%s]", fid, string(input))
	payload := &protos.Command_CallView{CallView: &protos.CallView{
		Fid:   fid,
		Input: input,
	}}
	sc, err := s.CreateSignedCommand(payload, s.SigningIdentity)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating signed command for [%s,%s]", fid, string(input))
	}

	ctx, span := s.tracer.Start(ctx, "ViewInvocation", tracing.WithAttributes(tracing.String("fid", fid)), trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	commandResp, err := s.processCommand(ctx, sc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed process command for [%s,%s]", fid, string(input))
	}

	if commandResp.GetCallViewResponse() == nil {
		return nil, errors.New("expected call view response, got nothing")
	}
	return commandResp.GetCallViewResponse().GetResult(), nil
}

// StreamCallView calls the given view with the given input and returns a stream to communicate with it.
//
// The client sends the call once and never retries it, because views are not
// idempotent. If the client cannot connect to the node, the call fails before
// the request is sent and the view does not run. If the call fails after the
// request is sent, for example when the connection drops or a deadline expires,
// the view may have run fully or partially, and the caller decides whether
// running it again is safe.
func (s *Client) StreamCallView(fid string, input []byte) (*Stream, error) {
	logger.Debugf("Streaming view call [%s] on input [%s]", fid, string(input))
	payload := &protos.Command_CallView{CallView: &protos.CallView{
		Fid:   fid,
		Input: input,
	}}
	sc, err := s.CreateSignedCommand(payload, s.SigningIdentity)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating signed command for [%s,%s]", fid, string(input))
	}

	closer, scc, err := s.streamCommand(context.Background(), sc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed stream command for [%s,%s]", fid, string(input))
	}

	return &Stream{scc: scc, closer: closer, sc: sc}, nil
}

// processCommand sends sc and returns the decoded response, which must answer sc.
func (s *Client) processCommand(ctx context.Context, sc *protos.SignedCommand) (*protos.CommandResponse, error) {
	svc, closer, err := s.Transport.Open()
	if err != nil {
		return nil, errors.Wrap(err, "failed creating view client")
	}
	defer func() { _ = closer.Close() }()

	logger.Debugf("process command [%s]", logging.SHA256Base64(sc.Command))
	scr, err := svc.ProcessCommand(ctx, sc)
	if err != nil {
		return nil, errors.Wrap(err, "failed view client process command")
	}
	return decodeResponse(sc, scr)
}

// decodeResponse decodes scr, checks that it answers sc, and turns an err response into an
// error wrapping view.ErrViewExecutionFailed. The response signature is not verified: TLS
// authenticates the node.
func decodeResponse(sc *protos.SignedCommand, scr *protos.SignedCommandResponse) (*protos.CommandResponse, error) {
	commandResp := &protos.CommandResponse{}
	if err := proto.Unmarshal(scr.Response, commandResp); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal command response")
	}
	want := sha256.Sum256(sc.Command)
	if !bytes.Equal(commandResp.GetHeader().GetCommandHash(), want[:]) {
		return nil, errors.New("response does not answer the command sent")
	}
	if commandResp.GetErr() != nil {
		return nil, errors.Wrapf(view.ErrViewExecutionFailed, "error from view during process command: %s", commandResp.GetErr().GetMessage())
	}
	return commandResp, nil
}

// streamCommand opens a stream and sends sc on it. The returned closer releases the
// transport once the stream is done.
func (s *Client) streamCommand(ctx context.Context, sc *protos.SignedCommand) (io.Closer, protos.ViewService_StreamCommandClient, error) {
	svc, closer, err := s.Transport.Open()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed creating view client")
	}
	scc, err := svc.StreamCommand(ctx)
	if err != nil {
		_ = closer.Close()
		return nil, nil, errors.Wrap(err, "failed view client stream command")
	}
	if err := scc.Send(sc); err != nil {
		_ = scc.CloseSend()
		_ = closer.Close()
		return nil, nil, errors.Wrap(err, "failed to send signed command")
	}
	return closer, scc, nil
}

func (s *Client) CreateSignedCommand(payload any, signingIdentity SigningIdentity) (*protos.SignedCommand, error) {
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
	tlsCertHash, err := grpc.GetTLSCertHash(s.Transport.Certificate())
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

func commandFromPayload(payload any) (*protos.Command, error) {
	switch t := payload.(type) {
	case *protos.Command_CallView:
		return &protos.Command{Payload: t}, nil
	default:
		return nil, errors.Wrapf(view.ErrCommandNotRecognized, "command type not recognized: %T", t)
	}
}
