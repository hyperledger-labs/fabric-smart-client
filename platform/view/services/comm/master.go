/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"encoding/base64"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func (p *P2PNode) getOrCreateSession(sessionID, endpointAddress, contextID, callerViewID string, caller view.Identity, endpointID []byte, msg *view.Message) (*NetworkStreamSession, error) {
	p.sessionsMutex.Lock()
	defer p.sessionsMutex.Unlock()

	internalSessionID := computeInternalSessionID(sessionID, endpointID)
	logger.Debugf("looking up session [%s]", internalSessionID)
	if session, in := p.sessions[internalSessionID]; in {
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
		middleCh:        make(chan *view.Message, DefaultIncomingMessagesBufferSize),
		closing:         make(chan struct{}),
		closed:          make(chan struct{}),
	}
}

// ReplyError sends payload as an error to the party with the given pkid on the session with
// sessionID, through a one-shot session that is never registered. Messages for sessionID are
// therefore not routed to it, and a registered session with that ID, such as one a responder
// is about to create or already uses, is not touched.
func (p *P2PNode) ReplyError(ctx context.Context, sessionID, contextID, endpoint string, pkid, payload []byte) error {
	s := p.newNetworkStreamSession(sessionID, endpoint, contextID, "", nil, pkid)
	defer s.Close()
	return s.SendError(ctx, payload)
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

// DeleteSession closes the session identified by sessionID and the remote party's pkid, and
// removes it from the registry. It does nothing if no such session exists.
//
// The lookup uses the exact registry key: a responder session's ID is chosen by the remote
// peer, so it must never select sessions that belong to other peers or the master session.
//
// The session is closed after the registry lock is released: closing waits for the session's
// consumer, and the dispatcher takes the lock for every incoming message.
func (p *P2PNode) DeleteSession(_ context.Context, sessionID string, pkid []byte) {
	p.sessionsMutex.Lock()
	key := computeInternalSessionID(sessionID, pkid)
	session, ok := p.sessions[key]
	if ok {
		delete(p.sessions, key)
		p.m.Sessions.Set(float64(len(p.sessions)))
	}
	p.sessionsMutex.Unlock()
	if !ok {
		return
	}
	logger.Debugf("deleting session [%s]", key)
	session.closeInternal()
}
