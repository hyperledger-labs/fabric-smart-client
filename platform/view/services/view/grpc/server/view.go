/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"encoding/json"
	"log"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

const fidLabel tracing.LabelName = "fid"

type viewHandler struct {
	viewManager ViewManager
	tracer      trace.Tracer
}

func InstallViewHandler(viewManager ViewManager, server Service, tracerProvider tracing.Provider) {
	fh := &viewHandler{
		viewManager: viewManager,
		tracer: tracerProvider.Tracer("view_handler", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "viewsdk",
			LabelNames: []tracing.LabelName{fidLabel, successLabel},
		})),
	}
	server.RegisterProcessor(reflect.TypeOf(&protos.Command_InitiateView{}), fh.initiateView)
	server.RegisterProcessor(reflect.TypeOf(&protos.Command_CallView{}), fh.callView)
	server.RegisterStreamer(reflect.TypeOf(&protos.Command_CallView{}), fh.streamCallView)
}

func (s *viewHandler) initiateView(ctx context.Context, command *protos.Command) (interface{}, error) {
	initiateView := command.Payload.(*protos.Command_InitiateView).InitiateView
	_, span := s.tracer.Start(ctx, "initiate_view", tracing.WithAttributes(tracing.String(fidLabel, initiateView.Fid)), trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	fid := initiateView.Fid
	input := initiateView.Input
	log.Printf("Initiate view [%s]", fid)

	f, err := s.viewManager.NewView(fid, input)
	if err != nil {
		return nil, errors.Errorf("failed instantiating view [%s], err [%s]", fid, err)
	}
	contextID, err := s.RunView(s.viewManager, f)
	if err != nil {
		return nil, errors.Errorf("failed running view [%s], err %s", fid, err)
	}
	return &protos.CommandResponse_InitiateViewResponse{InitiateViewResponse: &protos.InitiateViewResponse{
		Cid: contextID,
	}}, nil
}

func (s *viewHandler) callView(ctx context.Context, command *protos.Command) (interface{}, error) {
	callView := command.Payload.(*protos.Command_CallView).CallView
	// newCtx, span := s.tracer.Start(ctx, "call_view", tracing.WithAttributes(tracing.String(fidLabel, callView.Fid)), trace.WithSpanKind(trace.SpanKindInternal))
	// defer span.End()
	fid := callView.Fid
	input := callView.Input
	logger.DebugfContext(ctx, "Call view [%s] on input [%v]", fid, string(input))

	f, err := s.viewManager.NewView(fid, input)
	if err != nil {
		return nil, errors.Errorf("failed instantiating view [%s], err [%s]", fid, err)
	}
	logger.DebugfContext(ctx, "Initiate new view")
	result, err := s.viewManager.InitiateView(f, ctx)

	if err != nil {
		return nil, errors.Errorf("failed running view [%s], err %s", fid, err)
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
	callView := command.Payload.(*protos.Command_CallView).CallView

	fid := callView.Fid
	input := callView.Input
	logger.Debugf("Stream call view [%s] on input [%v]", fid, string(input))

	f, err := s.viewManager.NewView(fid, input)
	if err != nil {
		return errors.Errorf("failed instantiating view [%s], err [%s]", fid, err)
	}
	context, err := s.viewManager.InitiateContext(f)
	if err != nil {
		return errors.Errorf("failed running view [%s], err %s", fid, err)
	}
	mutable, ok := context.(view2.MutableContext)
	if !ok {
		return errors.Errorf("expected a mutable contexdt")
	}
	if err := mutable.PutService(&Stream{scs: commandServer}); err != nil {
		return errors.Errorf("failed registering stream command server")
	}

	result, err := context.RunView(f)
	if err != nil {
		return errors.Errorf("failed running view [%s], err %s", fid, err)
	}
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

func (s *viewHandler) RunView(manager ViewManager, view view2.View) (string, error) {
	context, err := manager.InitiateContext(view)
	if err != nil {
		return "", err
	}

	// Run the view
	go s.runView(view, context)

	return context.ID(), nil
}

func (s *viewHandler) runView(view view2.View, context view2.Context) {
	result, err := context.RunView(view)
	if err != nil {
		logger.Errorf("Failed view execution. Err [%s]\n", err)
	} else {
		logger.Debugf("Successful view execution. Result [%s]\n", result)
	}
}

type Stream struct {
	scs protos.ViewService_StreamCommandServer
}

func (c *Stream) Send(m interface{}) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	s := &protos.CallViewResponse{
		Result: raw,
	}
	return c.SendProtoMsg(s)
}

func (c *Stream) Recv(m interface{}) error {
	s := &protos.CallViewResponse{}
	if err := c.RecvProtoMsg(s); err != nil {
		return err
	}
	return json.Unmarshal(s.Result, m)
}

func (c *Stream) SendProtoMsg(m interface{}) error {
	return c.scs.SendMsg(m)
}

func (c *Stream) RecvProtoMsg(m interface{}) error {
	return c.scs.RecvMsg(m)
}
