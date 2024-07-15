/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"runtime/debug"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.uber.org/zap/zapcore"
)

// NetworkStreamSession implements view.Session
type NetworkStreamSession struct {
	node            *P2PNode
	endpointID      []byte
	endpointAddress string
	contextID       string
	sessionID       string
	caller          view.Identity
	callerViewID    string
	incoming        chan *view.Message
	streams         map[*streamHandler]struct{}
	closed          bool
	mutex           sync.Mutex
}

func (n *NetworkStreamSession) Info() view.SessionInfo {
	n.mutex.Lock()
	ret := view.SessionInfo{
		ID:           n.sessionID,
		Caller:       n.caller,
		CallerViewID: n.callerViewID,
		Endpoint:     n.endpointAddress,
		EndpointPKID: n.endpointID,
		Closed:       n.closed,
	}
	n.mutex.Unlock()
	return ret
}

// Send sends the payload to the endpoint
func (n *NetworkStreamSession) Send(payload []byte) error {
	return n.SendWithContext(context.TODO(), payload)
}

func (n *NetworkStreamSession) SendWithContext(ctx context.Context, payload []byte) error {
	return n.sendWithStatus(ctx, payload, view.OK)
}

// SendError sends an error to the endpoint with the passed payload
func (n *NetworkStreamSession) SendError(payload []byte) error {
	return n.SendErrorWithContext(context.TODO(), payload)
}

func (n *NetworkStreamSession) SendErrorWithContext(ctx context.Context, payload []byte) error {
	return n.sendWithStatus(ctx, payload, view.ERROR)
}

// Receive returns a channel of messages received from the endpoint
func (n *NetworkStreamSession) Receive() <-chan *view.Message {
	return n.incoming
}

// Close releases all the resources allocated by this session
func (n *NetworkStreamSession) Close() {
	n.node.sessionsMutex.Lock()
	defer n.node.sessionsMutex.Unlock()
	n.close(context.Background())
}

func (n *NetworkStreamSession) close(ctx context.Context) {
	if n.closed {
		logger.Debugf("session [%s] already closed", n.sessionID)
		return
	}
	defer logger.Debugf("closing session [%s] done", n.sessionID)

	toClose := make([]*streamHandler, 0, len(n.streams))
	for stream := range n.streams {
		stream.sessionReferenceCount--
		if stream.sessionReferenceCount == 0 {
			toClose = append(toClose, stream)
		}
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("closing session [%d of %d] streams [%s], from [%s]", len(toClose), len(n.streams), n.sessionID, string(debug.Stack()))
	}
	for _, stream := range toClose {
		stream.close(ctx)
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("closing session incoming [%s]", n.sessionID)
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Debugf("closing incoming channel for session [%s] failed with error [%s]", n.sessionID, r)
			}
		}()
		close(n.incoming)
	}()

	info := host.StreamInfo{
		RemotePeerID:      string(n.endpointID),
		RemotePeerAddress: n.endpointAddress,
		ContextID:         n.contextID,
		SessionID:         n.sessionID,
	}
	n.node.closeStream(info, toClose)

	n.closed = true
	n.streams = make(map[*streamHandler]struct{})

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("closing session [%s] done", n.sessionID)
	}
}

func (n *NetworkStreamSession) sendWithStatus(ctx context.Context, payload []byte, status int32) error {
	info := host.StreamInfo{
		RemotePeerID:      string(n.endpointID),
		RemotePeerAddress: n.endpointAddress,
		ContextID:         n.contextID,
		SessionID:         n.sessionID,
	}
	err := n.node.sendTo(ctx, info, &ViewPacket{
		ContextID: n.contextID,
		SessionID: n.sessionID,
		Caller:    n.callerViewID,
		Status:    status,
		Payload:   payload,
	})
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("sent message [len:%d] to [%s:%s] with err [%s]", len(payload), string(n.endpointID), n.endpointAddress, err)
	}
	return err
}
