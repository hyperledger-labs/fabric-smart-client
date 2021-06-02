/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package comm

import (
	"encoding/base64"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func (p *P2PNode) getOrCreateSession(sessionID, endpointAddress, contextID, callerViewID string, caller view.Identity, endpointID []byte, msg *view.Message) (*NetworkStreamSession, error) {
	p.sessionsMutex.Lock()
	defer p.sessionsMutex.Unlock()

	internalSessionID := computeInternalSessionID(sessionID, endpointAddress, endpointID)
	logger.Debugf("looking up session [%s]", internalSessionID)
	if session, in := p.sessions[internalSessionID]; in {
		logger.Debugf("session [%s] exists, returning it", internalSessionID)
		session.callerViewID = callerViewID
		session.contextID = contextID
		session.caller = caller
		session.endpointAddress = endpointAddress
		session.endpointID = endpointID
		return session, nil
	}

	s := &NetworkStreamSession{
		endpointID:      endpointID,
		endpointAddress: endpointAddress,
		contextID:       contextID,
		callerViewID:    callerViewID,
		caller:          caller,
		sessionID:       sessionID,
		node:            p,
		incoming:        make(chan *view.Message, 1),
		streams:         make(map[*streamHandler]struct{}),
	}

	if msg != nil {
		logger.Debugf("pushing first message to [%s], [%s]", internalSessionID, msg)
		s.incoming <- msg
	} else {
		logger.Debugf("no first message to push to [%s]", internalSessionID)
	}

	p.sessions[internalSessionID] = s

	logger.Infof("session [%s] as internal session [%s] ready", sessionID, internalSessionID)

	return s, nil
}

func (p *P2PNode) NewSession(callerViewID string, contextID string, endpoint string, pkid []byte) (view.Session, error) {
	logger.Infof("new p2p session [%s,%s,%s,%s]", callerViewID, contextID, endpoint, base64.StdEncoding.EncodeToString(pkid))
	ID, err := GetRandomNonce()
	if err != nil {
		return nil, err
	}

	return p.getOrCreateSession(base64.StdEncoding.EncodeToString(ID), endpoint, contextID, callerViewID, nil, pkid, nil)
}

func (p *P2PNode) NewSessionWithID(sessionID, contextID, endpoint string, pkid []byte, caller view.Identity, msg *view.Message) (view.Session, error) {
	return p.getOrCreateSession(sessionID, endpoint, contextID, "", caller, pkid, msg)
}

func (p *P2PNode) MasterSession() (view.Session, error) {
	return p.getOrCreateSession(masterSession, "", "", "", nil, []byte{}, nil)
}
