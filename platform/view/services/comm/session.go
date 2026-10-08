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

	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ErrSessionClosed is returned when a message is sent when the session is closed.
var ErrSessionClosed = errors.New("session closed")

const DefaultDrainTimeout = 500 * time.Millisecond

type sender interface {
	sendTo(ctx context.Context, info host.StreamInfo, msg proto.Message, session *NetworkStreamSession) error
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

	startOnce sync.Once
	middleCh  chan *view.Message
	closing   chan struct{}
	closed    chan struct{}
	isClosing atomic.Bool
}

func (n *NetworkStreamSession) tryStart() {
	n.startOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()

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
				case <-ticker.C:
					n.cleanupStreams()
				}
			}
		}()
	})
}

func (n *NetworkStreamSession) cleanupStreams() {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	for sh := range n.streams {
		if sh.isClosed() {
			logger.Debugf("session [%s] pruning closed stream [%s]", n.sessionID, sh.stream.Hash())
			sh.release()
			delete(n.streams, sh)
		}
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

// Close releases all the resources allocated by this session
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
	n.finishClose()
}

// closeAsync marks the session closing before it returns, so the session refuses every later
// message, and finishes closing in the background, because that waits for the forwarder to drain.
func (n *NetworkStreamSession) closeAsync() {
	if n.isClosing.Swap(true) {
		return
	}
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
		if logger.IsEnabledFor(zapcore.DebugLevel) {
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

	n.mutex.RLock()
	info := host.StreamInfo{
		RemotePeerID:      string(n.endpointID),
		RemotePeerAddress: n.endpointAddress,
		ContextID:         n.contextID,
		SessionID:         n.sessionID,
	}
	packet := &ViewPacket{
		ContextID: n.contextID,
		SessionID: n.sessionID,
		Caller:    n.callerViewID,
		Status:    status,
		Payload:   payload,
	}
	n.mutex.RUnlock()

	err := n.node.sendTo(ctx, info, packet, n)
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
