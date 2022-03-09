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

func (d *Transaction) Delete(db string, key string) error {
	return d.dataTx.Delete(db, key)
}

func (d *Transaction) SingAndClose() ([]byte, error) {
	return d.dataTx.SingAndClose()
}

func (d *Transaction) Commit(b bool) (string, *types.TxReceiptResponseEnvelope, error) {
	return d.dataTx.Commit(b)
}

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

func (s *Session) NewTransaction(txID string) (*Transaction, error) {
	dataTx, err := s.s.DataTx(txID)
	if err != nil {
		return nil, err
	}
	return &Transaction{dataTx: dataTx}, nil
}

func (s *Session) QueryExecutor(db string) (*SessionQueryExecutor, error) {
	dataTx, err := s.s.DataTx("")
	if err != nil {
		return nil, err
	}
	return &SessionQueryExecutor{dataTx: dataTx, db: db}, nil
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
