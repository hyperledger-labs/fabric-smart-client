/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"io"
	"sync"

	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Session encapsulates a communication channel to an endpoint.
//
//go:generate counterfeiter -o mock/session.go -fake-name Session . Session
type Session = view.Session

// Sessions is responsible for managing a set of sessions.
type Sessions struct {
	s  map[string]view.Session
	mu sync.RWMutex
}

func newSessions() *Sessions {
	return &Sessions{s: map[string]view.Session{}}
}

// PutDefault registers a session as the default session for the given party.
func (s *Sessions) PutDefault(party view.Identity, session view.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.s[party.UniqueID()] = session
}

// Get returns the session for the given view ID and party.
func (s *Sessions) Get(viewId string, party view.Identity) view.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := lookupKey(viewId, party)
	session := s.s[key]
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Sessions.Get [%s] found [%t]", key, session != nil)
	}
	return session
}

// GetFirstOpen returns the first open session for the given view ID and list of parties.
func (s *Sessions) GetFirstOpen(viewId string, parties iterators.Iterator[view.Identity]) (view.Session, view.Identity) {
	defer parties.Close()
	var targetId view.Identity
	for party, err := parties.Next(); !errors.Is(err, io.EOF); party, err = parties.Next() {
		if err != nil {
			continue
		}

		session := s.Get(viewId, party)
		if session != nil {
			if session.Info().Closed {
				logger.Debugf("removing session [%s:%s], it is closed", viewId, party)
				s.Delete(viewId, party)
				return nil, party
			}
			logger.Debugf("session for [%s] found with identifier [%s]", party, viewId)
			return session, party
		}
		targetId = party
	}
	return nil, targetId
}

// Put registers a session for the given view ID and party.
func (s *Sessions) Put(viewId string, party view.Identity, session view.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := lookupKey(viewId, party)
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Sessions.Put [%s] found [%t]", key, session != nil)
	}
	s.s[key] = session
}

// Delete removes the session for the given view ID and party.
func (s *Sessions) Delete(viewId string, party view.Identity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.s, lookupKey(viewId, party))
}

// Reset removes all registered sessions.
func (s *Sessions) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.s = map[string]view.Session{}
}

// GetSessionIDs returns the IDs of all registered sessions.
func (s *Sessions) GetSessionIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.s))
	for _, session := range s.s {
		ids = append(ids, session.Info().ID)
	}
	return ids
}

func lookupKey(viewId string, party view.Identity) string {
	return viewId + party.UniqueID()
}
