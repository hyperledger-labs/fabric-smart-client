/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ErrViewTimedOut is the error a view call ends with when the view outlives the view
// service's timeout. Its text is fixed, so a client sees the same message on every transport
// and nothing of the view's own error.
var ErrViewTimedOut = errors.New("view execution timed out")

type viewHandler struct {
	viewManager ViewManager
	timeout     time.Duration
}

// InstallViewHandler registers the CallView command on server: as a unary processor, which
// runs the view and returns its result, and as a streamer, which also lets the view exchange
// messages with the client over the stream (see [Stream]). Both bound the view's context by
// timeout, and a view still running when it expires ends with [ErrViewTimedOut]; a
// non-positive timeout imposes no bound. Views must observe their context for the timeout to
// take effect; a streamed view blocked in [Stream.Recv] waits for the client regardless.
func InstallViewHandler(viewManager ViewManager, server Service, timeout time.Duration) {
	fh := &viewHandler{viewManager: viewManager, timeout: timeout}
	server.RegisterProcessor(reflect.TypeFor[*protos.Command_CallView](), fh.callView)
	server.RegisterStreamer(reflect.TypeFor[*protos.Command_CallView](), fh.streamCallView)
}

func (s *viewHandler) callView(ctx context.Context, command *protos.Command) (any, error) {
	payload, ok := command.Payload.(*protos.Command_CallView)
	if !ok {
		return nil, errors.Errorf("unexpected command payload type [%T]", command.Payload)
	}
	callView := payload.CallView
	fid := callView.Fid
	input := callView.Input
	logger.DebugfContext(ctx, "Call view [%s] on input [%v]", fid, string(input))

	f, err := s.viewManager.NewView(fid, input)
	if err != nil {
		return nil, errors.Wrapf(view.ErrViewInstantiationFailed, "failed instantiating view [%s]: %v", fid, err)
	}
	logger.DebugfContext(ctx, "Initiate new view")
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	result, err := s.viewManager.InitiateView(ctx, f)
	if err != nil {
		return nil, viewErr(ctx, fid, err)
	}
	raw, ok := result.([]byte)
	if !ok {
		raw, err = json.Marshal(result)
		if err != nil {
			return nil, errors.Errorf("failed marshalling result produced by view [%s], err [%s]", fid, err)
		}
	}

	logger.Debugf("Finished call view [%s] on input [%v]", fid, string(input))
	return &protos.CommandResponse_CallViewResponse{CallViewResponse: &protos.CallViewResponse{
		Result: raw,
	}}, nil
}

func (s *viewHandler) streamCallView(sc *protos.SignedCommand, command *protos.Command, commandServer protos.ViewService_StreamCommandServer, marshaller Marshaller) error {
	payload, ok := command.Payload.(*protos.Command_CallView)
	if !ok {
		return errors.Errorf("unexpected command payload type [%T]", command.Payload)
	}
	callView := payload.CallView

	fid := callView.Fid
	input := callView.Input
	logger.Debugf("Stream call view [%s] on input [%v]", fid, string(input))

	f, err := s.viewManager.NewView(fid, input)
	if err != nil {
		return errors.Wrapf(view.ErrViewInstantiationFailed, "failed instantiating view [%s]: %v", fid, err)
	}
	ctx, cancel := s.withTimeout(commandServer.Context())
	defer cancel()
	viewCtx, err := s.viewManager.InitiateContext(ctx, f)
	if err != nil {
		return errors.Wrapf(view.ErrViewExecutionFailed, "failed running view [%s]: %v", fid, err)
	}
	mutable, ok := viewCtx.(view2.MutableContext)
	if !ok {
		s.viewManager.DeleteContext(viewCtx.ID())
		return errors.New("expected a mutable context")
	}
	if err := mutable.PutService(&Stream{scs: commandServer}); err != nil {
		s.viewManager.DeleteContext(viewCtx.ID())
		return errors.Wrap(err, "failed registering stream command server")
	}

	result, err := viewCtx.RunView(f)
	if err != nil {
		s.viewManager.DeleteContext(viewCtx.ID())
		return viewErr(ctx, fid, err)
	}
	defer s.viewManager.DeleteContext(viewCtx.ID())
	raw, ok := result.([]byte)
	if !ok {
		raw, err = json.Marshal(result)
		if err != nil {
			return errors.Errorf("failed marshalling result produced by view [%s], err [%s]", fid, err)
		}
	}
	logger.Debugf("Finished stream call view [%s] on input [%v]", fid, string(input))
	logger.Debugf("Preparing response")
	cr, err := marshaller.MarshalCommandResponse(
		sc.Command,
		&protos.CommandResponse_CallViewResponse{CallViewResponse: &protos.CallViewResponse{
			Result: raw,
		}},
	)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal command response for [%s]", fid)
	}
	logger.Debugf("Done with err [%s]", err)
	return commandServer.Send(cr)
}

// withTimeout bounds ctx by the view timeout; a non-positive timeout imposes none.
func (s *viewHandler) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, s.timeout)
}

// viewErr maps a failure of view fid to the error returned to the client. The expiry of
// ctx's deadline (the view timeout, or an earlier deadline the client set) is a timeout; a
// view that fails with a deadline error of its own while ctx is live reports that failure.
func viewErr(ctx context.Context, fid string, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrViewTimedOut
	}
	return errors.Wrapf(view.ErrViewExecutionFailed, "failed running view [%s]: %v", fid, err)
}

type Stream struct {
	scs protos.ViewService_StreamCommandServer
}

func (c *Stream) Send(m any) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	s := &protos.CallViewResponse{
		Result: raw,
	}
	return c.SendProtoMsg(s)
}

func (c *Stream) Recv(m any) error {
	s := &protos.CallViewResponse{}
	if err := c.RecvProtoMsg(s); err != nil {
		return err
	}
	return json.Unmarshal(s.Result, m)
}

func (c *Stream) SendProtoMsg(m any) error {
	return c.scs.SendMsg(m)
}

func (c *Stream) RecvProtoMsg(m any) error {
	return c.scs.RecvMsg(m)
}
