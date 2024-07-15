/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"bufio"
	"context"
	"encoding/binary"
	io2 "io"
	"runtime/debug"
	"sync"

	"github.com/gogo/protobuf/io"
	"github.com/gogo/protobuf/proto"
	proto2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
)

const (
	masterSession                    = "master of puppets I'm pulling your strings"
	contextIDLabel tracing.LabelName = "context_id"
)

var errStreamNotFound = errors.New("stream not found")

var logger = flogging.MustGetLogger("view-sdk")

type messageWithStream struct {
	message *view.Message
	stream  *streamHandler
}

type P2PNode struct {
	host             host2.P2PHost
	incomingMessages chan *messageWithStream
	streamsMutex     sync.RWMutex
	streams          map[host2.StreamHash][]*streamHandler
	dispatchMutex    sync.Mutex
	sessionsMutex    sync.Mutex
	sessions         map[string]*NetworkStreamSession
	finderWg         sync.WaitGroup
	isStopping       bool
	tracer           trace.Tracer
}

func NewNode(h host2.P2PHost, tracerProvider trace.TracerProvider) (*P2PNode, error) {
	p := &P2PNode{
		host:             h,
		incomingMessages: make(chan *messageWithStream),
		streams:          make(map[host2.StreamHash][]*streamHandler),
		sessions:         make(map[string]*NetworkStreamSession),
		isStopping:       false,
		tracer: tracerProvider.Tracer("comm_node", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "viewsdk",
			LabelNames: []tracing.LabelName{contextIDLabel},
		})),
	}
	if err := h.Start(p.handleIncomingStream); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *P2PNode) Start(ctx context.Context) {
	go p.dispatchMessages(ctx)
	go func() {
		<-ctx.Done()
		p.Stop()
	}()
}

func (p *P2PNode) Stop() {
	p.streamsMutex.Lock()
	p.isStopping = true
	p.streamsMutex.Unlock()

	p.host.Close()

	for _, streams := range p.streams {
		for _, stream := range streams {
			stream.close()
		}
	}

	close(p.incomingMessages)
	p.finderWg.Wait()
}

func (p *P2PNode) dispatchMessages(ctx context.Context) {
	for {
		select {
		case msg, ok := <-p.incomingMessages:
			newCtx, span := p.tracer.Start(msg.message.Ctx, "message_dispatch", tracing.WithAttributes(
				tracing.String(contextIDLabel, msg.message.ContextID)))
			msg.message.Ctx = newCtx
			if !ok {
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("channel closed, returning")
				}
				span.End()
				return
			}

			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("dispatch message from [%s,%s] on session [%s]", msg.message.FromEndpoint, view.Identity(msg.message.FromPKID).String(), msg.message.SessionID)
			}

			p.dispatchMutex.Lock()

			p.sessionsMutex.Lock()
			internalSessionID := computeInternalSessionID(msg.message.SessionID, msg.message.FromPKID)
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("dispatch message on internal session [%s]", internalSessionID)
			}
			session, in := p.sessions[internalSessionID]
			if in {
				span.AddEvent("reuse_session")
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("internal session exists [%s]", internalSessionID)
				}
				session.mutex.Lock()
				session.callerViewID = msg.message.Caller
				session.contextID = msg.message.ContextID
				session.endpointAddress = msg.message.FromEndpoint
				// here we know that msg.stream is used for session:
				// 1) increment the used counter for msg.stream
				msg.stream.refCtr++
				// 2) add msg.stream to the list of streams used by session
				session.streams[msg.stream] = struct{}{}
				session.mutex.Unlock()
			}
			p.sessionsMutex.Unlock()

			if !in {
				span.AddEvent("create_session")
				// create session but redirect this first message to master
				// _, _ = p.getOrCreateSession(
				//	msg.message.SessionID,
				//	msg.message.FromEndpoint,
				//	msg.message.ContextID,
				//	"",
				//	nil,
				//	msg.message.FromPKID,
				//	nil,
				// )

				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("internal session does not exists [%s], dispatching to master session", internalSessionID)
				}
				session, _ = p.getOrCreateSession(masterSession, "", "", "", nil, []byte{}, nil)
			}
			p.dispatchMutex.Unlock()

			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("pushing message to [%s], [%s]", internalSessionID, msg.message)
			}
			session.incoming <- msg.message
			span.End()
		case <-ctx.Done():
			logger.Info("closing p2p comm...")
			return
		}
	}
}

func (p *P2PNode) sendWithCachedStreams(streamHash string, msg proto.Message) error {
	if len(streamHash) == 0 {
		logger.Debugf("Empty stream hash probably because of uninitialized data. New stream must be created.")
		return errStreamNotFound
	}
	p.streamsMutex.RLock()
	defer p.streamsMutex.RUnlock()
	for _, stream := range p.streams[streamHash] {
		err := stream.send(msg)
		if err == nil {
			return nil
		}
		// TODO: handle the case in which there's an error
		logger.Errorf("error while sending message [%s] to stream with hash [%s]: %s", msg, streamHash, err)
	}

	return errStreamNotFound
}

// sendTo sends the passed messaged to the p2p peer with the passed ID.
// If no address is specified, then p2p will use one of the IP addresses associated to the peer in its peer store.
// If an address is specified, then the peer store will be updated with the passed address.
func (p *P2PNode) sendTo(ctx context.Context, info host2.StreamInfo, msg proto.Message) error {
	streamHash := p.host.StreamHash(info)
	if err := p.sendWithCachedStreams(streamHash, msg); err != errStreamNotFound {
		return err
	}

	nwStream, err := p.host.NewStream(ctx, info)
	if err != nil {
		return errors.Wrapf(err, "failed to create new stream to [%s]", info.RemotePeerID)
	}
	p.handleOutgoingStream(nwStream)

	return p.sendWithCachedStreams(nwStream.Hash(), msg)
}

func (p *P2PNode) handleOutgoingStream(stream host2.P2PStream) {
	p.handleStream(stream)
}

func (p *P2PNode) handleIncomingStream(stream host2.P2PStream) {
	p.handleStream(stream)
}

func (p *P2PNode) handleStream(stream host2.P2PStream) {
	sh := &streamHandler{
		stream: stream,
		reader: NewDelimitedReader(stream, 655360*2),
		writer: io.NewDelimitedWriter(stream),
		node:   p,
	}

	logger.Debugf("Adding new stream [%s]", stream.Hash())
	streamHash := stream.Hash()
	p.streamsMutex.Lock()
	p.streams[streamHash] = append(p.streams[streamHash], sh)
	p.streamsMutex.Unlock()

	go sh.handleIncoming()
}

func (p *P2PNode) Lookup(peerID string) ([]string, bool) {
	return p.host.Lookup(peerID)
}

type streamHandler struct {
	lock   sync.Mutex
	stream host2.P2PStream
	reader io.ReadCloser
	writer io.WriteCloser
	node   *P2PNode
	wg     sync.WaitGroup
	refCtr int
}

func (s *streamHandler) send(msg proto.Message) error {
	_, span := s.node.tracer.Start(s.stream.Context(), "message_send", tracing.WithAttributes(tracing.String(contextIDLabel, s.stream.ContextID())))
	defer span.End()
	s.lock.Lock()
	defer s.lock.Unlock()
	if err := s.writer.WriteMsg(msg); err != nil {
		span.RecordError(err)
		return err
	}
	return nil
}

func (s *streamHandler) handleIncoming() {
	s.wg.Add(1)
	for {
		msg := &ViewPacket{}
		err := s.reader.ReadMsg(msg)
		ctx, span := s.node.tracer.Start(s.stream.Context(), "message_receive", tracing.WithAttributes(tracing.String(contextIDLabel, msg.ContextID)))
		if err != nil {
			if s.node.isStopping {
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("error reading message while closing. ignoring.", err.Error())
				}
				span.End()
				break
			}

			logger.Debugf("error reading message: [%s][%s]", err.Error(), debug.Stack())

			// remove stream handler
			streamHash := s.node.host.StreamHash(host2.StreamInfo{
				RemotePeerID:      s.stream.RemotePeerID(),
				RemotePeerAddress: s.stream.RemotePeerAddress(),
				ContextID:         msg.ContextID,
				SessionID:         msg.SessionID,
			})
			s.node.streamsMutex.Lock()
			logger.Debugf("Removing stream [%s]. Total streams found: %d", streamHash, len(s.node.streams[streamHash]))
			for i, thisSH := range s.node.streams[streamHash] {
				if thisSH == s {
					s.node.streams[streamHash] = append(s.node.streams[streamHash][:i], s.node.streams[streamHash][i+1:]...)
					s.wg.Done()
					s.node.streamsMutex.Unlock()
					span.End()
					return
				}
			}
			s.node.streamsMutex.Unlock()

			span.End()
			// this should never happen!
			panic("couldn't find stream handler to remove")
		}
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("incoming message from [%s] on session [%s]", msg.Caller, msg.SessionID)
		}

		s.node.incomingMessages <- &messageWithStream{
			message: &view.Message{
				ContextID:    msg.ContextID,
				SessionID:    msg.SessionID,
				Status:       msg.Status,
				Payload:      msg.Payload,
				Caller:       msg.Caller,
				FromEndpoint: s.stream.RemotePeerAddress(),
				FromPKID:     []byte(s.stream.RemotePeerID()),
				Ctx:          ctx,
			},
			stream: s,
		}
		span.End()
	}
}

func (s *streamHandler) close() {
	_, span := s.node.tracer.Start(s.stream.Context(), "stream_close", tracing.WithAttributes(tracing.String(contextIDLabel, s.stream.ContextID())))
	defer span.End()
	s.reader.Close()
	s.writer.Close()
	s.stream.Close()
	// s.wg.Wait()
}

func NewDelimitedReader(r io2.Reader, maxSize int) io.ReadCloser {
	var closer io2.Closer
	if c, ok := r.(io2.Closer); ok {
		closer = c
	}
	return &varintReader{r: bufio.NewReader(r), closer: closer, maxSize: maxSize}
}

type varintReader struct {
	r       *bufio.Reader
	buf     []byte
	closer  io2.Closer
	maxSize int
}

func (r *varintReader) ReadMsg(msg proto.Message) error {
	length64, err := binary.ReadUvarint(r.r)
	if err != nil {
		return err
	}
	length := int(length64)
	if length < 0 {
		return io2.ErrShortBuffer
	}
	if len(r.buf) < length {
		r.buf = make([]byte, length)
	}
	if len(r.buf) >= r.maxSize {
		logger.Warnf("reading message length [%d]", length64)
	}
	buf := r.buf[:length]
	n, err := io2.ReadFull(r.r, buf)
	if err != nil {
		return errors.Wrapf(err, "error reading message of length [%d]", length)
	}
	if n != length {
		return errors.Errorf("failed to read [%d] bytes", length)
	}
	if err := proto2.Unmarshal(buf, msg); err != nil {
		return errors.Wrapf(err, "error unmarshalling message of length [%d]", length)
	}
	return nil
}

func (r *varintReader) Close() error {
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}
