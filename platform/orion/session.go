/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/orion-server/pkg/types"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
)

type Transaction struct {
	dataTx driver.DataTx
}

func (d *Transaction) Put(db string, key string, bytes []byte, a *types.AccessControl) error {
	return d.dataTx.Put(db, key, bytes, a)
}

func (d *Transaction) Get(db string, key string) ([]byte, *types.Metadata, error) {
	return d.dataTx.Get(db, key)
}

func (d *Transaction) Commit(b bool) (string, *types.TxReceipt, error) {
	return d.dataTx.Commit(b)
}

type QueryExecutor struct {
	dataTx driver.DataTx
	db     string
}

func (d *QueryExecutor) Get(key string) ([]byte, *types.Metadata, error) {
	return d.dataTx.Get(d.db, key)
}

type Session struct {
	s driver.Session
}

func (s *Session) Transaction() (*Transaction, error) {
	dataTx, err := s.s.DataTx()
	if err != nil {
		return nil, err
	}
	return &Transaction{dataTx: dataTx}, nil
}

func (s *Session) QueryExecutor(db string) (*QueryExecutor, error) {
	dataTx, err := s.s.DataTx()
	if err != nil {
		return nil, err
	}
	return &QueryExecutor{dataTx: dataTx, db: db}, nil
}

type SessionManager struct {
	sm driver.SessionManager
}

func (sm *SessionManager) NewSession(id string) (*Session, error) {
	s, err := sm.sm.NewSession(id)
	if err != nil {
		return nil, err
	}
	return &Session{s: s}, nil
}
