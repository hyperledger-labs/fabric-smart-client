/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	server2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	vidLabel tracing.LabelName = "vid"
)

// Option configures the view handler and client.
type Option func(*options)

type options struct {
	timeout time.Duration
}

// WithTimeout sets the execution timeout for view invocations. A duration
// less than or equal to zero imposes no deadline. Views must observe their
// context (e.g. via ctx.Done()) for the timeout to take effect.
func WithTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.timeout = timeout
	}
}

type viewCallFunc func(context *server2.ReqContext, vid string, input []byte) (any, error)

func (vcf viewCallFunc) CallView(context *server2.ReqContext, vid string, input []byte) (any, error) {
	return vcf(context, vid, input)
}

type viewHandler struct {
	c *client
}

func (s *viewHandler) CallView(context *server2.ReqContext, vid string, input []byte) (any, error) {
	result, err := s.c.CallView(vid, input, context.Req.Context())
	if err != nil {
		return nil, errors.Wrapf(errors.Join(view.ErrViewExecutionFailed, err), "failed running view [%s]", vid)
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

func (s *viewHandler) StreamCallView(context *server2.ReqContext, vid string, _ []byte) (any, error) {
	return nil, s.c.StreamCallView(vid, context.ResponseWriter, context.Req)
}

// InstallViewHandler installs the web view handler into the given HTTP handler.
// Callers can supply options, such as WithTimeout, to configure view execution limits.
// Views must observe their context (e.g. by selecting on ctx.Done()) for the timeout to
// take effect and abort execution early.
func InstallViewHandler(manager server.ViewManager, identityProvider server.IdentityProvider, h *server2.HttpHandler, tp tracing.Provider, opts ...Option) {
	fh := &viewHandler{c: newViewClient(manager, identityProvider, tp, opts...)}
	newDispatcher(h).WireViewCaller(viewCallFunc(fh.CallView))
	newDispatcher(h).WireStreamViewCaller(viewCallFunc(fh.StreamCallView))
}

// ViewClient defines an interface that can call views and stream calls.
type ViewClient interface {
	StreamCallView(fid string, writer http.ResponseWriter, request *http.Request) error
	CallView(fid string, in []byte, ctx context.Context) (any, error)
}

type client struct {
	viewManager      server.ViewManager
	identityProvider server.IdentityProvider
	tracer           trace.Tracer
	timeout          time.Duration
}

func newViewClient(viewManager server.ViewManager, identityProvider server.IdentityProvider, tp tracing.Provider, opts ...Option) *client {
	var o options
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return &client{
		viewManager:      viewManager,
		identityProvider: identityProvider,
		timeout:          o.timeout,
		tracer: tp.Tracer("view_client", tracing.WithMetricsOpts(tracing.MetricsOpts{
			LabelNames: []tracing.LabelName{vidLabel},
		})),
	}
}

// CallView calls the view with the given ID and input.
func (s *client) CallView(vid string, input []byte, ctx context.Context) (any, error) {
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}
	newCtx, span := s.tracer.Start(ctx, "call_view",
		tracing.WithAttributes(tracing.String(vidLabel, vid)),
		trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	logger.Debugf("Call view [%s] on input [%v]", vid, string(input))

	span.AddEvent("new_view")
	f, err := s.viewManager.NewView(vid, input)
	if err != nil {
		return nil, errors.Wrapf(view.ErrViewInstantiationFailed, "failed instantiating view [%s]: %v", vid, err)
	}
	span.AddEvent("initiate_view")
	raw, err := s.viewManager.InitiateView(newCtx, f)
	if err == nil {
		logger.Debugf("Finished call view [%s] on input [%v]", vid, string(input))
	}
	return raw, err
}

// StreamCallView calls the view with the given ID and input, and streams the communication over a web socket.
// A failed call closes the web socket with its error. Closing reads from the web socket, so the
// view must not use the stream, including from goroutines it started, once RunView returns.
func (s *client) StreamCallView(vid string, writer http.ResponseWriter, request *http.Request) (err error) {
	logger.Debugf("Call view [%s]", vid)

	// we need to retrieve the input to the factory from the web socket
	stream, err := server2.NewWSStream(writer, request)
	if err != nil {
		return errors.Wrapf(err, "failed to create web socket")
	}
	// The upgrade hijacks the connection, so net/http does not close it when the handler returns.
	defer func() { _ = stream.CloseWithError(err) }()
	input, err := stream.ReadInput()
	if err != nil {
		return errors.Wrapf(err, "failed to read input")
	}

	f, err := s.viewManager.NewView(vid, input)
	if err != nil {
		return errors.Wrapf(view.ErrViewInstantiationFailed, "failed instantiating view [%s]: %v", vid, err)
	}

	ctx := request.Context()
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}
	viewContext, err := s.viewManager.InitiateContext(ctx, f)
	if err != nil {
		return errors.Wrapf(view.ErrViewExecutionFailed, "failed instantiating context for view [%s]: %v", vid, err)
	}
	defer s.viewManager.DeleteContext(viewContext.ID())

	// register the web socket
	mutable, ok := viewContext.(view2.MutableContext)
	if !ok {
		return errors.Errorf("expected a mutable context")
	}
	if err := mutable.PutService(stream); err != nil {
		return errors.Errorf("failed registering stream command server")
	}
	// run the view
	result, err := viewContext.RunView(f)
	if err != nil {
		return errors.Wrapf(view.ErrViewExecutionFailed, "failed running view [%s]: %v", vid, err)
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
