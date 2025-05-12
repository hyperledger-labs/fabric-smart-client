/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
	"reflect"
	"runtime/debug"
	"strconv"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

const successLabel tracing.LabelName = "success"

var logger = logging.MustGetLogger()

//go:generate counterfeiter -o mock/marshaler.go -fake-name Marshaler . Marshaler

// A PolicyChecker is responsible for performing policy based access control
// checks related to view commands.
type PolicyChecker interface {
	Check(sc *protos2.SignedCommand, c *protos2.Command) error
}

// Server is responsible for processing view commands.
type Server struct {
	protos2.UnimplementedViewServiceServer
	Marshaller    Marshaller
	PolicyChecker PolicyChecker

	processors map[reflect.Type]Processor
	streamers  map[reflect.Type]Streamer
	metrics    *Metrics
	tracer     trace.Tracer
}

func NewViewServiceServer(marshaller Marshaller, policyChecker PolicyChecker, metrics *Metrics, tracerProvider trace.TracerProvider) (*Server, error) {
	return &Server{
		Marshaller:    marshaller,
		PolicyChecker: policyChecker,
		processors:    map[reflect.Type]Processor{},
		streamers:     map[reflect.Type]Streamer{},
		metrics:       metrics,
		tracer: tracerProvider.Tracer("view_service", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "viewsdk",
			LabelNames: []tracing.LabelName{successLabel},
		})),
	}, nil
}

func (s *Server) ProcessCommand(ctx context.Context, sc *protos2.SignedCommand) (cr *protos2.SignedCommandResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("ProcessCommand triggered panic: %s\n%s\n", r, debug.Stack())
			err = errors.Errorf("ProcessCommand triggered panic: %s", r)
		}
	}()

	logger.Debugf("Processes Command invoked...")

	command, err := UnmarshalCommand(sc.Command)
	if err != nil {
		return s.MarshalErrorResponse(sc.Command, err)
	}

	err = s.ValidateHeader(command.Header)
	if err != nil {
		return s.MarshalErrorResponse(sc.Command, err)
	}

	err = s.PolicyChecker.Check(sc, command)
	if err != nil {
		return s.MarshalErrorResponse(sc.Command, err)
	}

	labels := []string{"command", reflect.TypeOf(command.GetPayload()).String()}
	s.metrics.RequestsReceived.With(labels...).Add(1)
	defer func() {
		labels := append(labels, "success", strconv.FormatBool(err == nil))
		s.metrics.RequestsCompleted.With(labels...).Add(1)
	}()

	p, ok := s.processors[reflect.TypeOf(command.GetPayload())]
	var payload interface{}
	if ok {
		payload, err = p(ctx, command)
	} else {
		err = errors.Errorf("command type not recognized: %T", reflect.TypeOf(command.GetPayload()))
	}
	if err != nil {
		logger.Errorf("command execution failed with err [%s]", err)
		payload = &protos2.CommandResponse_Err{
			Err: &protos2.Error{Message: err.Error()},
		}
	}

	logger.Debugf("Preparing response")
	cr, err = s.Marshaller.MarshalCommandResponse(sc.Command, payload)
	logger.Debugf("Done with err [%s]", err)

	return
}

func (s *Server) StreamCommand(server protos2.ViewService_StreamCommandServer) error {
	sc := &protos2.SignedCommand{}
	if err := server.RecvMsg(sc); err != nil {
		return err
	}
	return s.streamCommand(sc, server)
}

func (s *Server) streamCommand(sc *protos2.SignedCommand, commandServer protos2.ViewService_StreamCommandServer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("ProcessCommand triggered panic: %s\n%s\n", r, debug.Stack())
			err = errors.Errorf("ProcessCommand triggered panic: %s\n%s\n", r, debug.Stack())
		}
	}()

	logger.Debugf("Stream Command invoked...")

	command, err := UnmarshalCommand(sc.Command)
	if err != nil {
		return s.streamError(err, sc, commandServer)
	}

	err = s.ValidateHeader(command.Header)
	if err != nil {
		return s.streamError(err, sc, commandServer)
	}

	err = s.PolicyChecker.Check(sc, command)
	if err != nil {
		return s.streamError(err, sc, commandServer)
	}

	streamer, ok := s.streamers[reflect.TypeOf(command.GetPayload())]
	switch ok {
	case true:
		logger.Debugf("got a streamer for [%s], invoke it...", reflect.TypeOf(command.GetPayload()))
		err = streamer(sc, command, commandServer, s.Marshaller)
	default:
		err = errors.Errorf("stream command type not recognized: %T", reflect.TypeOf(command.GetPayload()))
	}
	if err != nil {
		logger.Errorf("stream command execution failed with err [%s]", err)
		return s.streamError(err, sc, commandServer)
	}
	logger.Debugf("Stream Command invoked successfully")
	return nil
}

func (s *Server) ValidateHeader(header *protos2.Header) error {
	if header == nil {
		return errors.New("command header is required")
	}

	if len(header.Nonce) == 0 {
		return errors.New("nonce is required in header")
	}

	if len(header.Creator) == 0 {
		return errors.New("creator is required in header")
	}

	return nil
}

func (s *Server) MarshalErrorResponse(command []byte, e error) (*protos2.SignedCommandResponse, error) {
	return s.Marshaller.MarshalCommandResponse(
		command,
		&protos2.CommandResponse_Err{
			Err: &protos2.Error{Message: e.Error()},
		})
}

func (s *Server) RegisterProcessor(typ reflect.Type, p Processor) {
	s.processors[typ] = p
}

func (s *Server) RegisterStreamer(typ reflect.Type, streamer Streamer) {
	s.streamers[typ] = streamer
}

func (s *Server) streamError(err error, sc *protos2.SignedCommand, commandServer protos2.ViewService_StreamCommandServer) error {
	r, err2 := s.MarshalErrorResponse(sc.Command, err)
	if err2 != nil {
		return errors.WithMessagef(err, "failed creating resposse [%s]", err2)
	}
	err2 = commandServer.Send(r)
	if err2 != nil {
		return errors.WithMessagef(err, "failed creating resposse [%s]", err2)
	}
	logger.Errorf("stream error occurred [%s]", err)
	return err

}
