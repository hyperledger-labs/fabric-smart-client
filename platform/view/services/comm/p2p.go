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
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gogo/protobuf/io"
	"github.com/gogo/protobuf/proto"
	proto2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/protocol"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

const (
	viewProtocol     = "/fsc/view/1.0.0"
	rendezVousString = "fsc"
	masterSession    = "master of puppets I'm pulling your strings"
)

var errStreamNotFound = errors.New("stream not found")

var logger = flogging.MustGetLogger("view-sdk")

type messageWithStream struct {
	message *view.Message
	stream  *streamHandler
}

type P2PNode struct {
	host             host.Host
	dht              *dht.IpfsDHT
	finder           *routing.RoutingDiscovery
	peersMutex       sync.RWMutex
	peers            map[string]peer.AddrInfo
	incomingMessages chan *messageWithStream
	streamsMutex     sync.RWMutex
	streams          map[peer.ID][]*streamHandler
	dispatchMutex    sync.Mutex
	sessionsMutex    sync.Mutex
	sessions         map[string]*NetworkStreamSession
	stopFinder       int32
	finderWg         sync.WaitGroup
	isStopping       bool
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
	atomic.StoreInt32(&p.stopFinder, 1)

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
			if !ok {
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("channel closed, returning")
				}
				return
			}

			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("dispatch message from [%s,%s] on session [%s]", msg.message.FromEndpoint, view.Identity(msg.message.FromPKID).String(), msg.message.SessionID)
			}

			p.dispatchMutex.Lock()

			p.sessionsMutex.Lock()
			internalSessionID := computeInternalSessionID(msg.message.SessionID, msg.message.FromEndpoint, msg.message.FromPKID)
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("dispatch message on internal session [%s]", internalSessionID)
			}
			session, in := p.sessions[internalSessionID]
			if in {
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
		case <-ctx.Done():
			logger.Info("closing p2p comm...")
			return
		}
	}
}

func (p *P2PNode) sendWithCachedStreams(ID peer.ID, msg proto.Message) error {
	p.streamsMutex.RLock()
	defer p.streamsMutex.RUnlock()
	for _, stream := range p.streams[ID] {
		err := stream.send(msg)
		if err == nil {
			return nil
		}
		// TODO: handle the case in which there's an error
		logger.Errorf("error while sending message [%s] to peer [%s]: %s", msg, ID, err)
	}

	return errStreamNotFound
}

// sendTo sends the passed messaged to the libp2p peer with the passed ID.
// If no address is specified, then libp2p will use one of the IP addresses associated to the peer in its peer store.
// If an address is specified, then the peer store will be updated with the passed address.
func (p *P2PNode) sendTo(IDString string, address string, msg proto.Message) error {
	ID, err := peer.Decode(IDString)
	if err != nil {
		return err
	}

	if err := p.sendWithCachedStreams(ID, msg); err != errStreamNotFound {
		return err
	}

	if len(address) != 0 && strings.HasPrefix(address, "/ip4/") {
		// reprogram the addresses of the peer before opening a new stream, if it is not in the right form yet
		ps := p.host.Peerstore()
		current := ps.Addrs(ID)

		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("sendTo, reprogram address [%s:%s]", IDString, address)
			for _, m := range current {
				logger.Debugf("sendTo, current address [%s:%s]", IDString, m.String())
			}
		}

		ps.ClearAddrs(ID)
		addr, err := AddressToEndpoint(address)
		if err != nil {
			return errors.WithMessagef(err, "failed to parse endpoint's address [%s]", address)
		}
		s, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			return errors.Wrapf(err, "failed to get mutliaddr for [%s]", address)
		}
		ps.AddAddr(ID, s, peerstore.OwnObservedAddrTTL)
	}

	nwStream, err := p.host.NewStream(context.Background(), ID, protocol.ID(viewProtocol))
	if err != nil {
		return errors.Wrapf(err, "failed to create new stream to [%s]", ID)
	}

	p.handleStream()(nwStream)

	return p.sendWithCachedStreams(ID, msg)
}

func (p *P2PNode) handleStream() network.StreamHandler {
	return func(stream network.Stream) {
		sh := &streamHandler{
			stream: stream,
			reader: NewDelimitedReader(stream, 655360*2),
			writer: io.NewDelimitedWriter(stream),
			node:   p,
		}

		remotePeerID := sh.stream.Conn().RemotePeer()
		p.streamsMutex.Lock()
		p.streams[remotePeerID] = append(p.streams[remotePeerID], sh)
		p.streamsMutex.Unlock()

		go sh.handleIncoming()
	}
}

func (p *P2PNode) Lookup(peerID string) (peer.AddrInfo, bool) {
	p.peersMutex.RLock()
	defer p.peersMutex.RUnlock()

	peer, in := p.peers[peerID]
	return peer, in
}

type streamHandler struct {
	lock   sync.Mutex
	stream network.Stream
	reader io.ReadCloser
	writer io.WriteCloser
	node   *P2PNode
	wg     sync.WaitGroup
	refCtr int
}

func (s *streamHandler) send(msg proto.Message) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.writer.WriteMsg(msg)
}

func (s *streamHandler) handleIncoming() {
	s.wg.Add(1)
	for {
		msg := &ViewPacket{}
		err := s.reader.ReadMsg(msg)
		if err != nil {
			if s.node.isStopping {
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("error reading message while closing. ignoring.", err.Error())
				}
				break
			}

			logger.Debugf("error reading message: [%s][%s]", err.Error(), debug.Stack())

			// remove stream handler
			remotePeerID := s.stream.Conn().RemotePeer()
			s.node.streamsMutex.Lock()
			for i, thisSH := range s.node.streams[remotePeerID] {
				if thisSH == s {
					s.node.streams[remotePeerID] = append(s.node.streams[remotePeerID][:i], s.node.streams[remotePeerID][i+1:]...)
					s.wg.Done()
					s.node.streamsMutex.Unlock()
					return
				}
			}
			s.node.streamsMutex.Unlock()

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
				FromEndpoint: s.stream.Conn().RemoteMultiaddr().String(),
				FromPKID:     []byte(s.stream.Conn().RemotePeer().String()),
			},
			stream: s,
		}
	}
}

func (s *streamHandler) close() {
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
