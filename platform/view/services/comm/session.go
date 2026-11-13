/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ErrSessionClosed is returned when a message is sent when the session is closed.
var ErrSessionClosed = errors.New("session closed")

const DefaultDrainTimeout = 500 * time.Millisecond

// sessionStreamGrace is how long a session whose last stream died waits for a replacement
// stream, opened by a send or by the remote, before it closes.
const sessionStreamGrace = 5 * time.Second

type sender interface {
	sendTo(ctx context.Context, info host.StreamInfo, msg proto.Message, session *NetworkStreamSession) error
	// streamPreamble returns the preamble the peer sent on a live stream for info, opening one
	// if there is none, and leases that stream to session. It is nil for a peer that sends none.
	streamPreamble(ctx context.Context, info host.StreamInfo, session *NetworkStreamSession) (*preamble.Preamble, error)
	// unregister removes session from the node's registry, if it is registered. selfClosed
	// tells that the session closed on its own rather than by a Close call.
	unregister(session *NetworkStreamSession, selfClosed bool)
}

// NetworkStreamSession implements view.Session
type NetworkStreamSession struct {
	node            sender
	endpointID      []byte
	localPKID       []byte
	endpointAddress string
	contextID       string
	sessionID       string
	caller          view.Identity
	callerViewID    string
	incoming        chan *view.Message
	streams         map[*streamHandler]struct{}
	mutex           sync.RWMutex
	// streamGrace is how long the session waits for a replacement stream, see sessionStreamGrace.
	streamGrace time.Duration
	// orphanTimer closes the session when it stays without a stream for streamGrace. Guarded by mutex.
	orphanTimer *time.Timer

	startOnce sync.Once
	middleCh  chan *view.Message
	closing   chan struct{}
	closed    chan struct{}
	isClosing atomic.Bool

	// initiator is set on a session this node opened with NewSession or NewSessionWithID: it
	// chooses the versions and states them, with protocol, on its messages. It never changes.
	initiator bool

	// versionMu guards the fields below. negotiate holds it while it opens the stream, so a
	// concurrent Send or Version on the session waits for that, bounded by the first caller's
	// context.
	versionMu sync.Mutex
	// protocol is the protocol name the messages state, see SetProtocol.
	protocol string
	// versions holds the version per component the session uses, 0 for a component without a
	// common version. On an initiator session it is nil until negotiate ran; on a responder
	// session it is what the message that created the session stated.
	versions map[string]uint32
	// peer is the preamble negotiate chose from, nil if the peer sent none; it names the peer's
	// range when a component has no common version.
	peer *preamble.Preamble
	// refusal is the error of the first local component without a common version, which every
	// message of an initiator session fails with; nil if every component has one.
	refusal error
}

// SetProtocol sets the protocol name "<component>/<name>" the session's messages state, so that
// the peer finds the responder registered for it. It has an effect only on a session this node
// opened. The responder is chosen by the message that reaches it first, so a name set after the
// first Send may not take effect.
func (n *NetworkStreamSession) SetProtocol(name string) {
	n.versionMu.Lock()
	defer n.versionMu.Unlock()
	n.protocol = name
}

// Version implements view.VersionedSession.
func (n *NetworkStreamSession) Version(ctx context.Context, component string) (uint32, bool, error) {
	local, ok := preamble.LocalRange(component)
	if !ok {
		return 0, false, nil
	}
	if n.isClosed() {
		return 0, false, errors.Wrapf(ErrSessionClosed, "session [%s] is closed", n.sessionID)
	}
	versions, peer, err := n.negotiate(ctx)
	if err != nil {
		return 0, false, err
	}
	v, stated := versions[component]
	switch {
	case !stated:
		// The initiator runs a release without this component, or without negotiation.
		v = preamble.DefaultRange.Max
	case v == 0 && n.initiator:
		return 0, false, noCommonVersion(component, local, peer)
	case v == 0:
		return 0, false, errors.Wrapf(view.ErrNoCommonVersion, "component [%s]: the initiator supports no version in local [%d, %d]", component, local.Min, local.Max)
	}
	if !local.Contains(v) {
		return 0, false, errors.Wrapf(view.ErrNoCommonVersion, "component [%s]: peer uses %d, local [%d, %d]", component, v, local.Min, local.Max)
	}
	return v, true, nil
}

// negotiate returns the session's versions and the preamble they were chosen from. On an
// initiator session the first call chooses, per local component, the highest version of both
// nodes' ranges, from the preamble of the session's stream, which it opens if needed; a failure
// to open it is returned, and the next call tries again.
func (n *NetworkStreamSession) negotiate(ctx context.Context) (map[string]uint32, *preamble.Preamble, error) {
	n.versionMu.Lock()
	defer n.versionMu.Unlock()
	if !n.initiator || n.versions != nil {
		return n.versions, n.peer, nil
	}
	peer, err := n.node.streamPreamble(ctx, n.streamInfo(), n)
	if err != nil {
		return nil, nil, errors.WithMessagef(err, "failed negotiating versions for session [%s]", n.sessionID)
	}
	versions := map[string]uint32{}
	for _, c := range preamble.Components() {
		v, ok := preamble.Highest(c.Range, preamble.PeerRange(peer, c.Name))
		if !ok {
			v = 0 // marks that the nodes support no common version
			if n.refusal == nil {
				n.refusal = noCommonVersion(c.Name, c.Range, peer)
			}
		}
		versions[c.Name] = v
	}
	n.versions, n.peer = versions, peer
	return versions, peer, nil
}

// noCommonVersion returns the error of an initiator session for component, which has no
// version in both local, this node's range, and the range in peer, the peer's preamble.
func noCommonVersion(component string, local preamble.Range, peer *preamble.Preamble) error {
	r := preamble.PeerRange(peer, component)
	return errors.Wrapf(view.ErrNoCommonVersion, "component [%s]: local [%d, %d], peer [%d, %d]", component, local.Min, local.Max, r.Min, r.Max)
}

// stated returns the versions and protocol a message states: on an initiator session, every
// message states them, so that the responder learns them from whichever message reaches it
// first, even one that arrives before the responder registered the session; on a responder
// session, none. On an initiator session with a component that has no common version, it fails
// as Version does for that component, so that no message is sent: a responder of a release
// that checks versions would reject the session, and one of an older release would not check.
func (n *NetworkStreamSession) stated(ctx context.Context) ([]byte, string, error) {
	if !n.initiator {
		return nil, "", nil
	}
	versions, _, err := n.negotiate(ctx)
	if err != nil {
		return nil, "", err
	}
	n.versionMu.Lock()
	defer n.versionMu.Unlock()
	if n.refusal != nil {
		return nil, "", n.refusal
	}
	raw, err := preamble.EncodeVersions(versions)
	if err != nil {
		return nil, "", err
	}
	return raw, n.protocol, nil
}

// lease registers sh as a stream of the session, unless it already is or the session closes:
// a closing session releases its streams once, so a lease taken later would never be released.
func (n *NetworkStreamSession) lease(sh *streamHandler) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	if _, ok := n.streams[sh]; !ok && !n.isClosing.Load() && sh.tryLease() {
		n.streams[sh] = struct{}{}
	}
}

func (n *NetworkStreamSession) tryStart() {
	n.startOnce.Do(func() {
		go func() {
			exit := func(v *view.Message, needSend bool) {
				drainTimeout := DefaultDrainTimeout
				if needSend {
					select {
					case n.incoming <- v:
					case <-time.After(drainTimeout):
						logger.Warnf("dropping last message for session [%s] on exit, consumer not responding", n.sessionID)
					}
				}
				for {
					select {
					case mv := <-n.middleCh:
						select {
						case n.incoming <- mv:
						case <-time.After(drainTimeout):
							logger.Warnf("dropping message for session [%s] on exit, consumer not responding", n.sessionID)
							goto out
						}
					default:
						goto out
					}
				}
			out:
				close(n.closed)
				close(n.incoming)
			}

			for {
				select {
				case <-n.closing:
					exit(nil, false)
					return
				case v := <-n.middleCh:
					select {
					case <-n.closing:
						exit(v, true)
						return
					case n.incoming <- v:
					}
				}
			}
		}()
	})
}

// dropDeadStream removes sh, whose reads failed, from the session. A session left without a
// stream closes streamGrace after its last stream died, unless a replacement stream attaches in
// the meantime, so a reader waiting on Receive() sees the closed channel instead of waiting for
// a message that cannot arrive. Each time the session loses its last stream, the full grace
// period starts again. The master session never closes this way.
func (n *NetworkStreamSession) dropDeadStream(sh *streamHandler) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	if _, held := n.streams[sh]; !held {
		return
	}
	sh.release()
	delete(n.streams, sh)
	if len(n.streams) > 0 || n.isMaster() {
		return
	}
	if n.orphanTimer == nil {
		n.orphanTimer = time.AfterFunc(n.streamGrace, n.closeIfOrphaned)
	} else {
		n.orphanTimer.Reset(n.streamGrace)
	}
}

// isMaster tells whether n is the master session. The registry key identifies it, not the session
// ID: a remote peer can choose the ID.
func (n *NetworkStreamSession) isMaster() bool {
	return computeInternalSessionID(n.sessionID, n.endpointID) == masterSessionKey
}

func (n *NetworkStreamSession) closeIfOrphaned() {
	n.mutex.RLock()
	orphaned := len(n.streams) == 0
	n.mutex.RUnlock()
	if orphaned {
		logger.Debugf("session [%s] has no live stream, closing", n.sessionID)
		n.closeAsync()
	}
}

// Info returns a view.SessionInfo.
func (n *NetworkStreamSession) Info() view.SessionInfo {
	n.mutex.RLock()
	defer n.mutex.RUnlock()
	ret := view.SessionInfo{
		ID:             n.sessionID,
		Caller:         n.caller,
		CallerViewID:   n.callerViewID,
		RemoteEndpoint: n.endpointAddress,
		RemotePKID:     n.endpointID,
		LocalPKID:      n.localPKID,
		Closed:         n.isClosed(),
	}
	return ret
}

// Send sends the payload to the endpoint with the passed context.Context.
func (n *NetworkStreamSession) Send(ctx context.Context, payload []byte) error {
	return n.sendWithStatus(ctx, payload, view.OK)
}

// SendError sends an error to the endpoint with the passed context.Context and payload.
func (n *NetworkStreamSession) SendError(ctx context.Context, payload []byte) error {
	return n.sendWithStatus(ctx, payload, view.ERROR)
}

// Receive returns a channel of messages received from the endpoint
func (n *NetworkStreamSession) Receive() <-chan *view.Message {
	return n.incoming
}

// enqueue hands msg to the session's forwarder without blocking, so that one session whose
// consumer stopped draining cannot hold up the node's dispatcher, which serves every session.
// It returns false, dropping msg, when the session is closing or closed, or when its queue is
// full. It does not close the session.
func (n *NetworkStreamSession) enqueue(msg *view.Message) bool {
	if msg == nil {
		logger.Debugf("nil message provided for session [%s]", n.sessionID)
		return false
	}
	logger.Debugf("enqueue called for session [%s] with message len %d", n.sessionID, len(msg.Payload))

	if n.isClosing.Load() {
		logger.Debugf("session [%s] is closing, refusing to enqueue message", n.sessionID)
		return false
	}

	// let's try to start the session
	n.tryStart()

	select {
	case <-n.closed:
		logger.Debugf("session [%s] is closed, refusing to enqueue message", n.sessionID)
		return false
	case n.middleCh <- msg:
		logger.Debugf("Successfully enqueued message for session [%s] via middleCh", n.sessionID)
		return true
	default:
		logger.Debugf("session [%s] queue full, refusing to enqueue message", n.sessionID)
		return false
	}
}

// Close releases all the resources allocated by this session and removes it from the node's
// registry.
func (n *NetworkStreamSession) Close() {
	n.closeInternal()
}

func (n *NetworkStreamSession) closeInternal() {
	if n.isClosing.Swap(true) {
		// Another close is in progress: wait for it, so that the session's streams are released
		// when this returns.
		<-n.closed
		n.closeStreams()
		return
	}
	n.node.unregister(n, false)
	n.finishClose()
}

// closeAsync closes a session that cannot continue: its consumer stopped draining or it lost its
// streams. It marks the session closing and removes it from the registry before it returns, so the
// session refuses every later message, and finishes closing in the background, because that waits
// for the forwarder to drain.
func (n *NetworkStreamSession) closeAsync() {
	if n.isClosing.Swap(true) {
		return
	}
	n.node.unregister(n, true)
	go n.finishClose()
}

func (n *NetworkStreamSession) finishClose() {
	// ensure the session is started so that the closing channel has a listener
	n.tryStart()

	select {
	case n.closing <- struct{}{}:
		<-n.closed
		n.closeStreams()
		logger.Debugf("closing session [%s] done", n.sessionID)
	case <-n.closed:
		n.closeStreams()
	}
}

// closeStreams releases the session's stream leases. It is idempotent: the released streams
// are removed, and a closing session takes no new leases.
func (n *NetworkStreamSession) closeStreams() {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	logger.Debugf("closing session [%s] with [%d] streams", n.sessionID, len(n.streams))
	for stream := range n.streams {
		if logger.IsEnabledFor(logging.DebugLevel) {
			logger.Debugf("session [%s], stream [%s], refCtr [%d]", n.sessionID, stream.stream.Hash(), stream.refCtr.Load())
		}
		stream.release()
	}

	logger.Debugf("closing session [%s]'s streams done", n.sessionID)
	clear(n.streams)
}

func (n *NetworkStreamSession) isClosed() bool {
	select {
	case <-n.closed:
		return true
	default:
	}

	return false
}

func (n *NetworkStreamSession) sendWithStatus(ctx context.Context, payload []byte, status int32) error {
	if n.isClosed() {
		return errors.Wrapf(ErrSessionClosed, "session [%s] is closed", n.sessionID)
	}

	versions, protocol, err := n.stated(ctx)
	if err != nil {
		return err
	}
	info := n.streamInfo()
	n.mutex.RLock()
	packet := &ViewPacket{
		ContextID: n.contextID,
		SessionID: n.sessionID,
		Caller:    n.callerViewID,
		Status:    status,
		Payload:   payload,
		Versions:  versions,
		Protocol:  protocol,
	}
	n.mutex.RUnlock()

	err = n.node.sendTo(ctx, info, packet, n)
	logger.Debugf("[%s] sent message [len:%d] to [%s:%s] from [%s] [status:%v] with err [%v]",
		n.sessionID,
		len(payload),
		info.RemotePeerID,
		info.RemotePeerAddress,
		packet.Caller,
		status,
		err,
	)
	if err != nil {
		return errors.Wrapf(err, "failed to send message on session [%s]", n.sessionID)
	}
	return nil
}

// streamInfo returns where the session's messages go.
func (n *NetworkStreamSession) streamInfo() host.StreamInfo {
	n.mutex.RLock()
	defer n.mutex.RUnlock()
	return host.StreamInfo{
		RemotePeerID:      string(n.endpointID),
		RemotePeerAddress: n.endpointAddress,
		ContextID:         n.contextID,
		SessionID:         n.sessionID,
	}
}
