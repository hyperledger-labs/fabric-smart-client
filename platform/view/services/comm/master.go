/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"encoding/base64"
	"strings"

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
		session.callerViewID = callerViewID
		session.contextID = contextID
		if len(caller) != 0 {
			if len(session.caller) == 0 {
				session.caller = caller
			} else if !session.caller.Equal(caller) {
				session.mutex.Unlock()
				return nil, errors.Errorf("caller identity mismatch for session [%s]", internalSessionID)
			}
		}
		session.endpointAddress = endpointAddress
		session.endpointID = endpointID
		session.mutex.Unlock()
		return session, nil
	}

	s := &NetworkStreamSession{
		node:            p,
		endpointID:      endpointID,
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

func (p *P2PNode) NewSession(callerViewID string, contextID string, endpoint string, pkid []byte) (view.Session, error) {
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

func (p *P2PNode) DeleteSessions(_ context.Context, sessionID string) {
	p.sessionsMutex.Lock()
	defer p.sessionsMutex.Unlock()

	for key, session := range p.sessions {
		// if key starts with sessionID, delete it
		if strings.HasPrefix(key, sessionID) {
			logger.Debugf("deleting session [%s]", key)
			session.closeInternal()
			delete(p.sessions, key)
		}
	}
	p.m.Sessions.Set(float64(len(p.sessions)))
}
