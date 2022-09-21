/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"sync"

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
	return n.sendWithStatus(payload, view.OK)
}

// SendError sends an error to the endpoint with the passed payload
func (n *NetworkStreamSession) SendError(payload []byte) error {
	return n.sendWithStatus(payload, view.ERROR)
}

// Receive returns a channel of messages received from the endpoint
func (n *NetworkStreamSession) Receive() <-chan *view.Message {
	return n.incoming
}

// Close releases all the resources allocated by this session
func (n *NetworkStreamSession) Close() {
	defer logger.Debugf("Closing session [%s]", n.sessionID)
	n.node.sessionsMutex.Lock()
	toClose := make([]*streamHandler, 0, len(n.streams))
	for stream := range n.streams {
		stream.refCtr--
		if stream.refCtr == 0 {
			toClose = append(toClose, stream)
		}
	}
	n.node.sessionsMutex.Unlock()

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Closing session stream [%s]", n.sessionID)
	}
	for _, stream := range toClose {
		stream.close()
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Closing session incoming [%s]", n.sessionID)
	}
	close(n.incoming)
	n.closed = true

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Closing session [%s] done", n.sessionID)
	}
}

func (n *NetworkStreamSession) sendWithStatus(payload []byte, status int32) error {
	err := n.node.sendTo(string(n.endpointID), n.endpointAddress, &ViewPacket{
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
