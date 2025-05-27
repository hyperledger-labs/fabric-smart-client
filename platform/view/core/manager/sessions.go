/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package manager

import (
	"io"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type Sessions struct {
	s  map[string]view.Session
	mu sync.RWMutex
}

func newSessions() *Sessions {
	return &Sessions{s: map[string]view.Session{}}
}

func (s *Sessions) Lock() {
	s.mu.Lock()
}

func (s *Sessions) Unlock() {
	s.mu.Unlock()
}

func (s *Sessions) PutDefault(party view.Identity, session view.Session) {
	s.s[party.UniqueID()] = session
}

func (s *Sessions) Get(viewId string, party view.Identity) view.Session {
	return s.s[lookupKey(viewId, party)]
}

func (s *Sessions) GetFirstOpen(viewId string, parties iterators.Iterator[view.Identity]) (view.Session, view.Identity) {
	defer parties.Close()
	var targetId view.Identity
	for party, err := parties.Next(); !errors.Is(err, io.EOF); party, err = parties.Next() {
		if err != nil {
			continue
		}

		session := s.Get(viewId, party)
		if session != nil && session.Info().Closed {
			logger.Debugf("removing session [%s:%s], it is closed", viewId, party)
			s.Delete(viewId, party)
			return nil, party
		}
		if session != nil {
			logger.Debugf("session for [%s] found with identifier [%s]", party, viewId)
			return session, party
		}
		targetId = party
	}
	return nil, targetId
}

func (s *Sessions) Put(viewId string, party view.Identity, session view.Session) {
	s.s[lookupKey(viewId, party)] = session
}

func (s *Sessions) Delete(viewId string, party view.Identity) {
	delete(s.s, lookupKey(viewId, party))
}

func (s *Sessions) Reset() {
	s.s = map[string]view.Session{}
}

func (s *Sessions) GetSessionIDs() []string {
	ids := make([]string, 0, len(s.s))
	for _, session := range s.s {
		ids = append(ids, session.Info().ID)
	}
	return ids
}

func lookupKey(viewId string, party view.Identity) string {
	return viewId + party.UniqueID()
}
