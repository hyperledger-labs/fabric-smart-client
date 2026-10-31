/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"crypto/sha256"
	"reflect"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
)

const successLabel tracing.LabelName = "success"

var logger = logging.MustGetLogger()

// A PolicyChecker is responsible for performing policy based access control checks related to view commands.
type PolicyChecker interface {
	Check(sc *protos.SignedCommand, c *protos.Command) error
}

// A BindingInspector receives as parameters a gRPC context and a proto message,
// and verifies whether the message contains an appropriate binding to the context.
type BindingInspector func(context.Context, proto.Message) error

// Server is responsible for processing view commands.
type Server struct {
	protos.UnimplementedViewServiceServer
	Marshaller       Marshaller
	PolicyChecker    PolicyChecker
	bindingInspector BindingInspector

	processors map[reflect.Type]Processor
	streamers  map[reflect.Type]Streamer
	metrics    *Metrics
	tracer     trace.Tracer
	timeWindow time.Duration
	accepted   *replayGuard
	// maxMessageSize bounds a SignedCommand and every later stream frame, in bytes; 0 is
	// no bound beyond the transport's own.
	maxMessageSize int
}

// ErrMessageTooLarge is the error a command or stream frame larger than the configured
// maximum ends with.
var ErrMessageTooLarge = errors.New("command exceeds fsc.view.remote.maxMessageSize")

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithMaxMessageSize bounds every SignedCommand, and every later frame of a stream, to n
// bytes; a non-positive n imposes no bound. The view service applies it whatever transport
// delivered the message, so it holds on every backend; a transport may stop reading earlier.
func WithMaxMessageSize(n int) ServerOption {
	return func(s *Server) { s.maxMessageSize = n }
}

// NewViewServiceServer returns a new instance of the view service server.
//
// The server accepts a command only if its header timestamp is within timeWindow of the
// local clock, and only once: it remembers every accepted command for twice timeWindow, the
// longest time a command stays acceptable. The memory is per process, so a restarted node, or
// another node holding the same identities, accepts a command again within its window.
func NewViewServiceServer(
	marshaller Marshaller,
	policyChecker PolicyChecker,
	metrics *Metrics,
	tracerProvider tracing.Provider,
	bindingInspector BindingInspector,
	timeWindow time.Duration,
	opts ...ServerOption,
) (*Server, error) {
	if timeWindow <= 0 {
		return nil, errors.Errorf("command time window must be positive, got [%s]", timeWindow)
	}
	s := &Server{
		Marshaller:       marshaller,
		PolicyChecker:    policyChecker,
		bindingInspector: bindingInspector,
		processors:       map[reflect.Type]Processor{},
		streamers:        map[reflect.Type]Streamer{},
		metrics:          metrics,
		timeWindow:       timeWindow,
		accepted:         &replayGuard{retention: 2 * timeWindow, seen: map[[sha256.Size]byte]struct{}{}},
		tracer: tracerProvider.Tracer("view_service", tracing.WithMetricsOpts(tracing.MetricsOpts{
			LabelNames: []tracing.LabelName{successLabel},
		})),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// tooLarge reports whether m exceeds the configured maximum message size.
func (s *Server) tooLarge(m proto.Message) bool {
	return s.maxMessageSize > 0 && proto.Size(m) > s.maxMessageSize
}

// limitedStream is a stream whose later frames are bounded like its command.
type limitedStream struct {
	protos.ViewService_StreamCommandServer
	s *Server
}

func (l limitedStream) Recv() (*protos.SignedCommand, error) {
	m := &protos.SignedCommand{}
	if err := l.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (l limitedStream) RecvMsg(m any) error {
	if err := l.ViewService_StreamCommandServer.RecvMsg(m); err != nil {
		return err
	}
	if pm, ok := m.(proto.Message); ok && l.s.tooLarge(pm) {
		return ErrMessageTooLarge
	}
	return nil
}

// ProcessCommand processes the passed command ensuring proper access control. A failure is
// returned as an error response, except a view cancelled by the node's shutdown, which ends the
// call with ErrAbortedByShutdown.
func (s *Server) ProcessCommand(ctx context.Context, sc *protos.SignedCommand) (cr *protos.SignedCommandResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("processCommand triggered panic: %s\n%s\n", r, debug.Stack())
			err = errors.Errorf("ProcessCommand triggered panic: %s", r)
		}
	}()

	logger.DebugfContext(ctx, "processCommand invoked...")

	if s.tooLarge(sc) {
		return s.MarshalErrorResponse(sc.Command, ErrMessageTooLarge)
	}
	command, err := UnmarshalCommand(sc.Command)
	if err != nil {
		return s.MarshalErrorResponse(sc.Command, err)
	}

	err = s.ValidateHeader(command.Header)
	if err != nil {
		return s.MarshalErrorResponse(sc.Command, err)
	}

	err = s.bindingInspector(ctx, command)
	if err != nil {
		return s.MarshalErrorResponse(sc.Command, err)
	}

	err = s.PolicyChecker.Check(sc, command)
	if err != nil {
		return s.MarshalErrorResponse(sc.Command, err)
	}

	err = s.checkReplay(sc)
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
	var payload any
	if ok {
		payload, err = p(ctx, command)
	} else {
		err = errors.Wrapf(view.ErrCommandNotRecognized, "command type not recognized: %T", reflect.TypeOf(command.GetPayload()))
	}
	if errors.Is(err, ErrAbortedByShutdown) {
		// A status, not an error response, so that the client tells it from a failed view.
		return nil, err
	}
	if err != nil {
		logger.ErrorfContext(ctx, "command execution failed with err [%s]", err.Error())
		payload = &protos.CommandResponse_Err{
			Err: &protos.Error{Message: err.Error()},
		}
	}

	logger.DebugfContext(ctx, "preparing response")
	cr, err = s.Marshaller.MarshalCommandResponse(sc.Command, payload)
	logger.DebugfContext(ctx, "done with err [%s]", err)

	return cr, err
}

// StreamCommand processes the passed streaming command ensuring proper access control. A
// failure is sent as an error response, except a view cancelled by the node's shutdown, which
// ends the stream with ErrAbortedByShutdown.
func (s *Server) StreamCommand(server protos.ViewService_StreamCommandServer) error {
	sc := &protos.SignedCommand{}
	if err := server.RecvMsg(sc); err != nil {
		return err
	}
	if s.tooLarge(sc) {
		return s.streamError(ErrMessageTooLarge, sc, server)
	}
	if s.maxMessageSize > 0 {
		server = limitedStream{ViewService_StreamCommandServer: server, s: s}
	}
	return s.handleStreamCommand(sc, server)
}

func (s *Server) handleStreamCommand(sc *protos.SignedCommand, commandServer protos.ViewService_StreamCommandServer) (err error) {
	ctx := commandServer.Context()
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("processCommand triggered panic: %s\n%s\n", r, debug.Stack())
			err = errors.Errorf("processCommand triggered panic: %s\n%s", r, debug.Stack())
		}
	}()

	logger.DebugfContext(ctx, "stream Command invoked...")

	command, err := UnmarshalCommand(sc.Command)
	if err != nil {
		return s.streamError(err, sc, commandServer)
	}

	err = s.ValidateHeader(command.Header)
	if err != nil {
		return s.streamError(err, sc, commandServer)
	}

	err = s.bindingInspector(ctx, command)
	if err != nil {
		return s.streamError(err, sc, commandServer)
	}

	err = s.PolicyChecker.Check(sc, command)
	if err != nil {
		return s.streamError(err, sc, commandServer)
	}

	err = s.checkReplay(sc)
	if err != nil {
		return s.streamError(err, sc, commandServer)
	}

	streamer, ok := s.streamers[reflect.TypeOf(command.GetPayload())]
	switch ok {
	case true:
		logger.DebugfContext(ctx, "got a streamer for [%s], invoke it...", reflect.TypeOf(command.GetPayload()))
		err = streamer(sc, command, commandServer, s.Marshaller)
	default:
		err = errors.Wrapf(view.ErrCommandNotRecognized, "stream command type not recognized: %T", reflect.TypeOf(command.GetPayload()))
	}
	if errors.Is(err, ErrAbortedByShutdown) {
		return err
	}
	if err != nil {
		logger.ErrorfContext(ctx, "stream command execution failed with err [%s]", err.Error())
		return s.streamError(err, sc, commandServer)
	}
	logger.DebugfContext(ctx, "stream Command invoked successfully")
	return nil
}

// ValidateHeader validates the given command header: it must carry a nonce, a creator, and a
// timestamp within the server's time window of the local clock.
func (s *Server) ValidateHeader(header *protos.Header) error {
	if header == nil {
		return errors.WithMessage(view.ErrCommandHeaderInvalid, "command header is required")
	}

	if err := header.GetTimestamp().CheckValid(); err != nil {
		return errors.WithMessagef(view.ErrCommandHeaderInvalid, "valid timestamp is required in header: %s", err)
	}
	if skew := time.Since(header.GetTimestamp().AsTime()).Abs(); skew > s.timeWindow {
		s.metrics.RequestsRejected.With("reason", "stale").Add(1)
		return errors.WithMessagef(view.ErrCommandHeaderInvalid, "timestamp is [%s] away from local time, more than the allowed [%s]", skew, s.timeWindow)
	}

	if len(header.Nonce) == 0 {
		return errors.WithMessage(view.ErrCommandHeaderInvalid, "nonce is required in header")
	}

	if len(header.Creator) == 0 {
		return errors.WithMessage(view.ErrCommandHeaderInvalid, "creator is required in header")
	}

	return nil
}

// checkReplay rejects a command the server has already accepted. It keys on the signed bytes,
// not the signature, because an ECDSA signature can be altered and still verify.
func (s *Server) checkReplay(sc *protos.SignedCommand) error {
	if !s.accepted.accept(sha256.Sum256(sc.Command), time.Now()) {
		s.metrics.RequestsRejected.With("reason", "replay").Add(1)
		return errors.WithMessage(view.ErrCommandHeaderInvalid, "command already processed")
	}
	return nil
}

// replayGuard remembers accepted command hashes for retention. Entries expire in insertion
// order, so each accept drops the expired prefix of the queue: amortized O(1) per call, with
// no background goroutine to stop.
type replayGuard struct {
	mu        sync.Mutex
	retention time.Duration
	seen      map[[sha256.Size]byte]struct{}
	queue     []replayEntry // ordered by acceptance
}

type replayEntry struct {
	key    [sha256.Size]byte
	expiry time.Time
}

// accept reports whether key is new at now, and records it. Callers pass non-decreasing
// times, as time.Now does.
func (g *replayGuard) accept(key [sha256.Size]byte, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for n < len(g.queue) && now.After(g.queue[n].expiry) {
		delete(g.seen, g.queue[n].key)
		n++
	}
	g.queue = g.queue[n:]
	if _, ok := g.seen[key]; ok {
		return false
	}
	g.seen[key] = struct{}{}
	g.queue = append(g.queue, replayEntry{key: key, expiry: now.Add(g.retention)})
	return true
}

// MarshalErrorResponse marshals the given error into a signed command response.
func (s *Server) MarshalErrorResponse(command []byte, e error) (*protos.SignedCommandResponse, error) {
	return s.Marshaller.MarshalCommandResponse(
		command,
		&protos.CommandResponse_Err{
			Err: &protos.Error{Message: e.Error()},
		})
}

// RegisterProcessor registers a processor for the given command payload type.
func (s *Server) RegisterProcessor(typ reflect.Type, p Processor) {
	s.processors[typ] = p
}

// RegisterStreamer registers a streamer for the given command payload type.
func (s *Server) RegisterStreamer(typ reflect.Type, streamer Streamer) {
	s.streamers[typ] = streamer
}

func (s *Server) streamError(err error, sc *protos.SignedCommand, commandServer protos.ViewService_StreamCommandServer) error {
	r, err2 := s.MarshalErrorResponse(sc.Command, err)
	if err2 != nil {
		return errors.WithMessagef(err, "failed creating response [%s]", err2)
	}
	err2 = commandServer.Send(r)
	if err2 != nil {
		return errors.WithMessagef(err, "failed creating response [%s]", err2)
	}
	logger.ErrorfContext(commandServer.Context(), "stream error occurred [%s]", err.Error())
	return err
}
