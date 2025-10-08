/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.uber.org/zap/zapcore"
)

// ErrSessionClosed is returned when a message is sent when the session is closed.
var ErrSessionClosed = errors.New("session closed")

type sender interface {
	sendTo(ctx context.Context, info host.StreamInfo, msg proto.Message) error
}

// NetworkStreamSession implements view.Session
type NetworkStreamSession struct {
	node            sender
	endpointID      []byte
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
}

func (n *NetworkStreamSession) tryStart() {
	n.startOnce.Do(func() {
		go func() {
			exit := func(v *view.Message, needSend bool) {
				close(n.closed)
				if needSend {
					n.incoming <- v
				}
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

// Info returns a view.SessionInfo.
func (n *NetworkStreamSession) Info() view.SessionInfo {
	n.mutex.RLock()
	defer n.mutex.RUnlock()
	ret := view.SessionInfo{
		ID:           n.sessionID,
		Caller:       n.caller,
		CallerViewID: n.callerViewID,
		Endpoint:     n.endpointAddress,
		EndpointPKID: n.endpointID,
		Closed:       n.isClosed(),
	}
	return ret
}

// Send sends the payload to the endpoint.
func (n *NetworkStreamSession) Send(payload []byte) error {
	return n.SendWithContext(context.TODO(), payload)
}

// SendWithContext sends the payload to the endpoint with the passed context.Context.
func (n *NetworkStreamSession) SendWithContext(ctx context.Context, payload []byte) error {
	return n.sendWithStatus(ctx, payload, view.OK)
}

// SendError sends an error to the endpoint with the passed payload.
func (n *NetworkStreamSession) SendError(payload []byte) error {
	return n.SendErrorWithContext(context.TODO(), payload)
}

// SendErrorWithContext sends an error to the endpoint with the passed context.Context and payload.
func (n *NetworkStreamSession) SendErrorWithContext(ctx context.Context, payload []byte) error {
	return n.sendWithStatus(ctx, payload, view.ERROR)
}

// Receive returns a channel of messages received from the endpoint
func (n *NetworkStreamSession) Receive() <-chan *view.Message {
	return n.incoming
}

// enqueue enqueues a message into the session's incoming channel.
// If the session is closed, the message will be dropped and false returned, otherwise true is returned.
func (n *NetworkStreamSession) enqueue(msg *view.Message) bool {
	if msg == nil {
		return false
	}

	// let's try to start the session
	n.tryStart()

	select {
	case <-n.closed:
		return false
	default:
	}

	select {
	case <-n.closed:
		return false
	case n.middleCh <- msg:
		return true
	}
}

// Close releases all the resources allocated by this session
func (n *NetworkStreamSession) Close() {
	n.closeInternal()
}

func (n *NetworkStreamSession) closeInternal() {
	closeStreams := func() {
		n.mutex.Lock()
		defer n.mutex.Unlock()

		logger.Debugf("closing session [%s] with [%d] streams", n.sessionID, len(n.streams))
		toClose := make([]*streamHandler, 0, len(n.streams))
		for stream := range n.streams {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("session [%s], stream [%s], refCtr [%d]", n.sessionID, stream.stream.Hash(), stream.refCtr)
			}
			stream.refCtr--
			if stream.refCtr == 0 {
				toClose = append(toClose, stream)
			}
		}

		logger.Debugf("closing session [%s]'s streams [%d]", n.sessionID, len(toClose))
		for _, stream := range toClose {
			stream.close(context.TODO())
		}
		logger.Debugf("closing session [%s]'s streams [%d] done", n.sessionID, len(toClose))
		clear(n.streams)
	}

	select {
	case n.closing <- struct{}{}:
		<-n.closed
		closeStreams()
		logger.Debugf("closing session [%s] done", n.sessionID)

	case <-n.closed:
	}
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
		return ErrSessionClosed
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

	err := n.node.sendTo(ctx, info, packet)
	logger.Debugf("sent message [len:%d] to [%s:%s] from [%s] [status:%v] with err [%v]",
		len(payload),
		info.RemotePeerID,
		info.RemotePeerAddress,
		packet.Caller,
		status,
		err,
	)
	return err
}
