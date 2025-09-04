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
	closed          bool
	closedCh        chan struct{}
	mutex           sync.RWMutex
	closeOnce       sync.Once
}

func newNetworkStreamSession(sessionID, contextID, endpointAddress string, endpointID []byte, callerViewID string, caller view.Identity, node sender) *NetworkStreamSession {
	return &NetworkStreamSession{
		endpointID:      endpointID,
		endpointAddress: endpointAddress,
		contextID:       contextID,
		callerViewID:    callerViewID,
		caller:          caller,
		sessionID:       sessionID,
		node:            node,
		incoming:        make(chan *view.Message, 1),
		streams:         make(map[*streamHandler]struct{}),
		closedCh:        make(chan struct{}),
	}
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
		Closed:       n.closed,
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

// received enqueues a message into the session's incoming channel.
// If the session is closed, the message will be dropped and a warning is logged.
func (n *NetworkStreamSession) received(msg *view.Message) {
	select {
	case <-n.closedCh:
		logger.Warnf("dropping message from %s for closed session [%s]", msg.Caller, msg.SessionID)
		return
	default:
	}

	select {
	case <-n.closedCh:
		logger.Warnf("dropping message from %s for closed session [%s]", msg.Caller, msg.SessionID)
		return
	case n.incoming <- msg:
	}
}

// Close releases all the resources allocated by this session
func (n *NetworkStreamSession) Close() {
	n.closeInternal()
}

func (n *NetworkStreamSession) closeInternal() {
	n.closeOnce.Do(func() {
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

		// next we are closing the incoming and the closing signal channel to drain the receivers;
		close(n.closedCh)
		close(n.incoming)
		n.closed = true
		n.streams = make(map[*streamHandler]struct{})

		logger.Debugf("closing session [%s] done", n.sessionID)
	})
}

func (n *NetworkStreamSession) sendWithStatus(ctx context.Context, payload []byte, status int32) error {
	select {
	case <-n.closedCh:
		// session already closed ...
		return errors.Wrapf(ErrSessionClosed, "sending failed")
	default:
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
