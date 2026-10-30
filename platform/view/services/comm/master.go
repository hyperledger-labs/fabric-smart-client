/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// masterSessionKey is the registry key of the master session.
var masterSessionKey = computeInternalSessionID(masterSession, nil)

func (p *P2PNode) getOrCreateSession(sessionID, endpointAddress, contextID, callerViewID string, caller view.Identity, endpointID []byte, msg *view.Message) (*NetworkStreamSession, error) {
	p.sessionsMutex.Lock()
	defer p.sessionsMutex.Unlock()

	internalSessionID := computeInternalSessionID(sessionID, endpointID)
	logger.Debugf("looking up session [%s]", internalSessionID)
	// A closing session is about to leave the registry, so it is replaced rather than returned. The
	// master session is never replaced: the view service holds the instance it was given.
	if session, in := p.sessions[internalSessionID]; in && (!session.isClosing.Load() || internalSessionID == masterSessionKey) {
		logger.Debugf("session [%s] exists, returning it", internalSessionID)
		session.mutex.Lock()
		// Validate the caller identity before touching any session state: a peer
		// presenting a mismatched identity must not be able to alter a session it
		// is not entitled to claim.
		if len(caller) != 0 && len(session.caller) != 0 && !session.caller.Equal(caller) {
			session.mutex.Unlock()
			return nil, errors.Errorf("caller identity mismatch for session [%s]", internalSessionID)
		}
		if len(caller) != 0 {
			session.caller = caller
		}
		session.callerViewID = callerViewID
		session.contextID = contextID
		session.endpointAddress = endpointAddress
		session.endpointID = endpointID
		session.mutex.Unlock()
		return session, nil
	}

	s := p.newNetworkStreamSession(sessionID, endpointAddress, contextID, callerViewID, caller, endpointID)

	if msg != nil {
		logger.Debugf("pushing first message to [%s], [%s]", internalSessionID, msg)
		if ok := s.enqueue(msg); !ok {
			logger.Errorf("can not enqueue message in newly created session [%s]", internalSessionID)
			return nil, errors.Errorf("can not enqueue message in newly created session [%s]", internalSessionID)
		}
	} else {
		logger.Debugf("no first message to push to [%s]", internalSessionID)
	}

	p.sessions[internalSessionID] = s
	delete(p.selfClosed, internalSessionID)
	p.m.Sessions.Set(float64(len(p.sessions)))

	s.tryStart()

	logger.Debugf("session [%s] as internal session [%s] ready", sessionID, internalSessionID)
	return s, nil
}

// newNetworkStreamSession returns a session that is neither registered nor started.
func (p *P2PNode) newNetworkStreamSession(sessionID, endpointAddress, contextID, callerViewID string, caller view.Identity, endpointID []byte) *NetworkStreamSession {
	return &NetworkStreamSession{
		node:            p,
		endpointID:      endpointID,
		localPKID:       []byte(p.host.PeerID()),
		endpointAddress: endpointAddress,
		contextID:       contextID,
		sessionID:       sessionID,
		caller:          caller,
		callerViewID:    callerViewID,
		incoming:        make(chan *view.Message, DefaultIncomingMessagesBufferSize),
		streams:         make(map[*streamHandler]struct{}),
		streamGrace:     sessionStreamGrace,
		middleCh:        make(chan *view.Message, DefaultIncomingMessagesBufferSize),
		closing:         make(chan struct{}),
		closed:          make(chan struct{}),
	}
}

// ReplyError sends payload as an error to the party with the given pkid on the session with
// sessionID. The packet has status view.ERROR and an empty caller, which the receiver's view
// service drops instead of answering, so rejections cannot loop between two nodes.
//
// ReplyError never registers a session: messages for sessionID are not routed to it, and a
// registered session with that ID, such as one a responder is about to create or already uses,
// is not touched. It also never opens a connection: it writes only to a stream to the peer
// that already exists (on WebSocket, the sub-connection the rejected message arrived on; on
// libp2p, any live stream to that peer), so that answering an unwanted message never costs a
// dial. If there is none, it returns an error wrapping errStreamNotFound. ctx is not consulted:
// the write is bounded by the transport's write deadline where it has one (WebSocket), and
// otherwise by the stream's lifetime (libp2p).
func (p *P2PNode) ReplyError(_ context.Context, sessionID, contextID, endpoint string, pkid, payload []byte) error {
	info := host.StreamInfo{RemotePeerID: string(pkid), RemotePeerAddress: endpoint, ContextID: contextID, SessionID: sessionID}
	packet := &ViewPacket{SessionID: sessionID, ContextID: contextID, Status: view.ERROR, Payload: payload}
	return p.sendWithCachedStreams(p.host.StreamHash(info), packet, nil)
}

func (p *P2PNode) NewSession(callerViewID, contextID, endpoint string, pkid []byte) (view.Session, error) {
	logger.Debugf("new p2p session [%s,%s,%s,%s]", callerViewID, contextID, endpoint, logging.Base64(pkid))
	ID, err := GetRandomNonce()
	if err != nil {
		return nil, err
	}

	return p.getOrCreateSession(base64.StdEncoding.EncodeToString(ID), endpoint, contextID, callerViewID, nil, pkid, nil)
}

func (p *P2PNode) NewResponderSession(sessionID, contextID, endpoint string, pkid []byte, caller view.Identity, msg *view.Message) (view.Session, error) {
	return p.getOrCreateSession(sessionID, endpoint, contextID, "", caller, pkid, msg)
}

func (p *P2PNode) NewSessionWithID(sessionID, contextID, endpoint string, pkid []byte) (view.Session, error) {
	return p.getOrCreateSession(sessionID, endpoint, contextID, "", nil, pkid, nil)
}

func (p *P2PNode) MasterSession() (view.Session, error) {
	return p.getOrCreateSession(masterSession, "", "", "", nil, []byte{}, nil)
}

// unregister removes session from the registry if it is the registered instance, so a newer
// session with the same key is never removed. The key of a session that closed on its own is
// remembered for closedSessionTTL, see dispatchMessages; the key of a session closed by Close is
// not, because its protocol is finished. The master session is never removed.
func (p *P2PNode) unregister(session *NetworkStreamSession, selfClosed bool) {
	if session.isMaster() {
		return
	}
	key := computeInternalSessionID(session.sessionID, session.endpointID)
	p.sessionsMutex.Lock()
	defer p.sessionsMutex.Unlock()
	if p.sessions[key] != session {
		return
	}
	delete(p.sessions, key)
	p.m.Sessions.Set(float64(len(p.sessions)))
	if !selfClosed {
		return
	}
	// ponytail: scans every remembered key per self-close; use a time-ordered queue if many sessions self-close at once.
	now := time.Now()
	for k, closedAt := range p.selfClosed {
		if now.Sub(closedAt) >= closedSessionTTL {
			delete(p.selfClosed, k)
		}
	}
	p.selfClosed[key] = now
}
