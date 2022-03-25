/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/orion-server/pkg/types"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
)

type SessionQueryExecutor struct {
	dataTx driver.DataTx
	db     string
}

func (d *SessionQueryExecutor) Get(key string) ([]byte, *types.Metadata, error) {
	return d.dataTx.Get(d.db, key)
}

type Session struct {
	s driver.Session
}

func (s *Session) QueryExecutor(db string) (*SessionQueryExecutor, error) {
	dataTx, err := s.s.DataTx("")
	if err != nil {
		return nil, err
	}
	return &SessionQueryExecutor{dataTx: dataTx, db: db}, nil
}

// SessionManager is a session manager that allows the developer to access orion directly
type SessionManager struct {
	sm driver.SessionManager
}

// NewSession creates a new session to orion using the passed identity
func (sm *SessionManager) NewSession(id string) (*Session, error) {
	s, err := sm.sm.NewSession(id)
	if err != nil {
		return nil, err
	}
	return &Session{s: s}, nil
}
