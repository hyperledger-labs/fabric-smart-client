/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"runtime/debug"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/io"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
)

const (
	masterSession                    = "master of puppets I'm pulling your strings"
	contextIDLabel tracing.LabelName = "context_id"
	sessionIDLabel tracing.LabelName = "session_id"
)

var errStreamNotFound = errors.New("stream not found")

var logger = logging.MustGetLogger()

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
	m                *Metrics
}

func NewNode(h host2.P2PHost, _ trace.TracerProvider, metricsProvider metrics.Provider) (*P2PNode, error) {
	p := &P2PNode{
		host:             h,
		incomingMessages: make(chan *messageWithStream),
		streams:          make(map[host2.StreamHash][]*streamHandler),
		sessions:         make(map[string]*NetworkStreamSession),
		isStopping:       false,
		m:                newMetrics(metricsProvider),
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

	utils.IgnoreErrorFunc(p.host.Close)

	p.streamsMutex.Lock()
	for _, streams := range p.streams {
		for _, stream := range streams {
			stream.close(context.Background())
		}
	}
	p.streamsMutex.Unlock()

	close(p.incomingMessages)
	p.finderWg.Wait()
}

func (p *P2PNode) dispatchMessages(ctx context.Context) {
	for {
		select {
		case msg, ok := <-p.incomingMessages:
			if !ok {
				logger.Debugf("channel closed, returning")
				return
			}

			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("dispatch message for context [%s] from [%s,%s] on session [%s]", msg.message.ContextID, msg.message.FromEndpoint, view.Identity(msg.message.FromPKID).String(), msg.message.SessionID)
			}

			p.dispatchMutex.Lock()

			p.sessionsMutex.Lock()
			internalSessionID := computeInternalSessionID(msg.message.SessionID, msg.message.FromPKID)
			logger.Debugf("dispatch message on internal session [%s]", internalSessionID)
			session, in := p.sessions[internalSessionID]
			if in {
				logger.Debugf("internal session exists [%s]", internalSessionID)
				session.mutex.Lock()
				session.callerViewID = msg.message.Caller
				session.contextID = msg.message.ContextID
				session.endpointAddress = msg.message.FromEndpoint
				// here we know that msg.stream is used for session:
				// 1) increment the used counter for msg.stream

				if _, streamRegisteredAlready := session.streams[msg.stream]; !streamRegisteredAlready {
					msg.stream.refCtr++
					// 2) add msg.stream to the list of streams used by session
					session.streams[msg.stream] = struct{}{}
				}
				session.mutex.Unlock()
			}
			p.sessionsMutex.Unlock()

			if !in {
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

				logger.Debugf("internal session does not exists [%s], dispatching to master session", internalSessionID)
				session, _ = p.getOrCreateSession(masterSession, "", "", "", nil, []byte{}, nil)
			}
			p.dispatchMutex.Unlock()

			logger.Debugf("pushing message to [%s], [%s]", internalSessionID, msg.message)
			session.incoming <- msg.message
		case <-ctx.Done():
			logger.Info("closing p2p comm...")
			return
		}
	}
}

func (p *P2PNode) sendWithCachedStreams(streamHash string, msg proto.Message) error {
	if len(streamHash) == 0 {
		logger.Debugf("empty stream hash probably because of uninitialized data. New stream must be created.")
		return errors.Wrapf(errStreamNotFound, "stream hash is empty")
	}
	p.streamsMutex.RLock()
	defer p.streamsMutex.RUnlock()
	logger.Debugf("send msg to stream hash [%s] of [%d] with #stream [%d]", streamHash, len(p.streams), len(p.streams[streamHash]))
	for _, stream := range p.streams[streamHash] {
		err := stream.send(msg)
		if err == nil {
			logger.Debugf("send msg [%v] with stream [%s]", msg, stream.stream.Hash())
			return nil
		}
		// TODO: handle the case in which there's an error
		logger.Errorf("error while sending message [%s] to stream with hash [%s]: %s", msg, streamHash, err)
	}

	return errors.Wrapf(errStreamNotFound, "all [%d] streams for hash [%s] failed to send", len(p.streams), streamHash)
}

// sendTo sends the passed messaged to the p2p peer with the passed ID.
// If no address is specified, then p2p will use one of the IP addresses associated to the peer in its peer store.
// If an address is specified, then the peer store will be updated with the passed address.
func (p *P2PNode) sendTo(ctx context.Context, info host2.StreamInfo, msg proto.Message) error {
	streamHash := p.host.StreamHash(info)
	if err := p.sendWithCachedStreams(streamHash, msg); !errors.Is(err, errStreamNotFound) {
		return errors.Wrap(err, "error while sending message to cached stream")
	}

	nwStream, err := p.host.NewStream(ctx, info)
	p.m.OpenedStreams.Add(1)
	if err != nil {
		return errors.Wrapf(err, "failed to create new stream to [%s]", info.RemotePeerID)
	}
	p.handleOutgoingStream(nwStream)

	err = p.sendWithCachedStreams(nwStream.Hash(), msg)
	if err != nil {
		return errors.Wrap(err, "error while sending message to freshly created stream")
	}
	return nil
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
		reader: io.NewDelimitedReader(stream, 655360*2),
		writer: io.NewDelimitedWriter(stream),
		node:   p,
	}

	streamHash := stream.Hash()
	p.streamsMutex.Lock()
	logger.Debugf(
		"adding new stream handler to hash [%s](of [%d]) with #handlers [%d]",
		streamHash,
		len(p.streams),
		len(p.streams[streamHash]),
	)
	p.streams[streamHash] = append(p.streams[streamHash], sh)
	p.m.StreamHashes.Set(float64(len(p.streams)))
	p.m.ActiveStreams.Add(1)
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
	s.lock.Lock()
	defer s.lock.Unlock()
	if err := s.writer.WriteMsg(msg); err != nil {
		return err
	}
	return nil
}

func (s *streamHandler) isStopping() bool {
	s.node.streamsMutex.RLock()
	defer s.node.streamsMutex.RUnlock()
	return s.node.isStopping
}

func (s *streamHandler) handleIncoming() {
	s.node.m.StreamHandlers.Add(1)
	defer s.node.m.StreamHandlers.Add(-1)
	s.wg.Add(1)
	for {
		msg := &ViewPacket{}
		err := s.reader.ReadMsg(msg)
		if err != nil {
			if s.isStopping() {
				logger.Debugf("error reading message while closing, ignoring [%s]", err)
				break
			}

			streamHash := s.stream.Hash()
			logger.Debugf("error reading message from stream [%s]: [%s][%s]", streamHash, err, debug.Stack())

			// remove stream handler
			s.node.streamsMutex.Lock()
			logger.Debugf("removing stream [%s], total streams found: %d", streamHash, len(s.node.streams[streamHash]))
			for i, thisSH := range s.node.streams[streamHash] {
				if thisSH == s {
					s.node.streams[streamHash] = append(s.node.streams[streamHash][:i], s.node.streams[streamHash][i+1:]...)
					if len(s.node.streams[streamHash]) == 0 {
						delete(s.node.streams, streamHash)
					}
					s.node.m.StreamHashes.Set(float64(len(s.node.streams)))
					s.node.m.ActiveStreams.Add(-1)
					s.wg.Done()
					s.node.streamsMutex.Unlock()
					s.close(context.Background())
					return
				}
			}
			s.node.streamsMutex.Unlock()

			// this should never happen!
			panic("couldn't find stream handler to remove")
		}
		logger.Debugf("incoming message for context [%s] from [%s] on session [%s]", msg.ContextID, msg.Caller, msg.SessionID)

		s.node.incomingMessages <- &messageWithStream{
			message: &view.Message{
				ContextID:    msg.ContextID,
				SessionID:    msg.SessionID,
				Status:       msg.Status,
				Payload:      msg.Payload,
				Caller:       msg.Caller,
				FromEndpoint: s.stream.RemotePeerAddress(),
				FromPKID:     []byte(s.stream.RemotePeerID()),
				Ctx:          s.stream.Context(),
			},
			stream: s,
		}
	}
}

func (s *streamHandler) close(context.Context) {
	if err := s.stream.Close(); err != nil {
		logger.Errorf("error closing stream [%s]: [%s]", s.stream.Hash(), err)
	}
	s.node.m.ClosedStreams.Add(1)
}
