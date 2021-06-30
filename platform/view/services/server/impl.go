/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"log"
	"reflect"
	"runtime/debug"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/protos"
)

var logger = flogging.MustGetLogger("view-sdk.server")

//go:generate counterfeiter -o mock/marshaler.go -fake-name Marshaler . Marshaler

// A PolicyChecker is responsible for performing policy based access control
// checks related to view commands.
type PolicyChecker interface {
	Check(sc *protos.SignedCommand, c *protos.Command) error
}

// Service is responsible for processing view commands.
type server struct {
	Marshaler     Marshaler
	PolicyChecker PolicyChecker

	processors map[reflect.Type]Processor
	streamers  map[reflect.Type]Streamer
}

func NewViewServiceServer(Marshaler Marshaler, PolicyChecker PolicyChecker) (*server, error) {
	return &server{
		Marshaler:     Marshaler,
		PolicyChecker: PolicyChecker,
		processors:    map[reflect.Type]Processor{},
		streamers:     map[reflect.Type]Streamer{},
	}, nil
}

func (s *server) ProcessCommand(ctx context.Context, sc *protos.SignedCommand) (cr *protos.SignedCommandResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ProcessCommand triggered panic: %s\n%s\n", r, debug.Stack())
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

	p, ok := s.processors[reflect.TypeOf(command.GetPayload())]
	var payload interface{}
	switch ok {
	case true:
		payload, err = p(ctx, command)
	default:
		err = errors.Errorf("command type not recognized: %T", reflect.TypeOf(command.GetPayload()))
	}
	if err != nil {
		logger.Errorf("command execution failed with err [%s]", err)
		payload = &protos.CommandResponse_Err{
			Err: &protos.Error{Message: err.Error()},
		}
	}

	logger.Debugf("Preparing response")
	cr, err = s.Marshaler.MarshalCommandResponse(sc.Command, payload)
	logger.Debugf("Done with err [%s]", err)

	return
}

func (s *server) StreamCommand(sc *protos.SignedCommand, commandServer protos.ViewService_StreamCommandServer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ProcessCommand triggered panic: %s\n%s\n", r, debug.Stack())
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
		err = streamer(sc, command, commandServer, s.Marshaler)
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

func (s *server) ValidateHeader(header *protos.Header) error {
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

func (s *server) MarshalErrorResponse(command []byte, e error) (*protos.SignedCommandResponse, error) {
	return s.Marshaler.MarshalCommandResponse(
		command,
		&protos.CommandResponse_Err{
			Err: &protos.Error{Message: e.Error()},
		})
}

func (s *server) RegisterProcessor(typ reflect.Type, p Processor) {
	s.processors[typ] = p
}

func (s *server) RegisterStreamer(typ reflect.Type, streamer Streamer) {
	s.streamers[typ] = streamer
}

func (s *server) streamError(err error, sc *protos.SignedCommand, commandServer protos.ViewService_StreamCommandServer) error {
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
