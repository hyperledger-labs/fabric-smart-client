/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	server2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

const (
	vidLabel tracing.LabelName = "vid"
)

type viewCallFunc func(context *server2.ReqContext, vid string, input []byte) (interface{}, error)

func (vcf viewCallFunc) CallView(context *server2.ReqContext, vid string, input []byte) (interface{}, error) {
	return vcf(context, vid, input)
}

type viewHandler struct {
	c *client
}

func (s *viewHandler) CallView(context *server2.ReqContext, vid string, input []byte) (interface{}, error) {
	result, err := s.c.CallView(vid, input, context.Req.Context())
	if err != nil {
		return nil, errors.Errorf("failed running view [%s], err %s", vid, err)
	}
	raw, ok := result.([]byte)
	if !ok {
		raw, err = json.Marshal(result)
		if err != nil {
			return nil, errors.Errorf("failed marshalling result produced by view [%s], err [%s]", vid, err)
		}
	}
	return &protos.CommandResponse_CallViewResponse{CallViewResponse: &protos.CallViewResponse{
		Result: raw,
	}}, nil
}

func (s *viewHandler) StreamCallView(context *server2.ReqContext, vid string, input []byte) (interface{}, error) {
	return nil, s.c.StreamCallView(vid, context.ResponseWriter, context.Req)
}

func InstallViewHandler(manager server.ViewManager, h *server2.HttpHandler, tp tracing.Provider) {
	fh := &viewHandler{c: newViewClient(manager, tp)}
	newDispatcher(h).WireViewCaller(viewCallFunc(fh.CallView))
	newDispatcher(h).WireStreamViewCaller(viewCallFunc(fh.StreamCallView))
}

type ViewClient interface {
	StreamCallView(fid string, writer http.ResponseWriter, request *http.Request) error
	CallView(fid string, in []byte, ctx context.Context) (interface{}, error)
}

type client struct {
	viewManager server.ViewManager
	tracer      trace.Tracer
}

func newViewClient(viewManager server.ViewManager, tp tracing.Provider) *client {
	return &client{
		viewManager: viewManager,
		tracer: tp.Tracer("view_client", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "viewsdk",
			LabelNames: []tracing.LabelName{vidLabel},
		})),
	}
}

func (s *client) CallView(vid string, input []byte, ctx context.Context) (interface{}, error) {
	newCtx, span := s.tracer.Start(ctx, "call_view",
		tracing.WithAttributes(tracing.String(vidLabel, vid)),
		trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	logger.Debugf("Call view [%s] on input [%v]", vid, string(input))

	span.AddEvent("new_view")
	f, err := s.viewManager.NewView(vid, input)
	if err != nil {
		return nil, errors.Errorf("failed instantiating view [%s], err [%s]", vid, err)
	}
	span.AddEvent("initiate_view")
	raw, err := s.viewManager.InitiateView(f, newCtx)
	if err == nil {
		logger.Debugf("Finished call view [%s] on input [%v]", vid, string(input))
	}
	return raw, err
}

func (s *client) StreamCallView(vid string, writer http.ResponseWriter, request *http.Request) error {
	logger.Debugf("Call view [%s]", vid)

	// we need to retrieve the input to the factory from the web socket
	stream, err := server2.NewWSStream(writer, request)
	if err != nil {
		return errors.Wrapf(err, "failed to create web socket")
	}
	input, err := stream.ReadInput()
	if err != nil {
		return errors.Wrapf(err, "failed to read input")
	}

	f, err := s.viewManager.NewView(vid, input)
	if err != nil {
		return errors.Errorf("failed instantiating view [%s], err [%s]", vid, err)
	}
	viewContext, err := s.viewManager.InitiateContext(f)
	if err != nil {
		return errors.Errorf("failed instantiating context for view [%s], err %s", vid, err)
	}

	// register the web socket
	mutable, ok := viewContext.(view2.MutableContext)
	if !ok {
		return errors.Errorf("expected a mutable contexdt")
	}
	if err := mutable.PutService(stream); err != nil {
		return errors.Errorf("failed registering stream command server")
	}
	// run the view
	result, err := viewContext.RunView(f)
	if err != nil {
		return errors.Errorf("failed running view [%s], err %s", vid, err)
	}
	raw, ok := result.([]byte)
	if !ok {
		raw, err = json.Marshal(result)
		if err != nil {
			return errors.Errorf("failed marshalling result produced by view [%s], err [%s]", vid, err)
		}
	}
	logger.Debugf("Finished call view [%s] on input [%v]", vid, string(input))

	// write back the result
	return stream.WriteResult(raw)
}
