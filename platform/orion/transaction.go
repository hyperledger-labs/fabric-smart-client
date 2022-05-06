/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/orion-server/pkg/types"
	"github.com/pkg/errors"
)

type TransientMap map[string][]byte

func (m TransientMap) Set(key string, raw []byte) error {
	m[key] = raw

	return nil
}

func (m TransientMap) Get(id string) []byte {
	return m[id]
}

func (m TransientMap) IsEmpty() bool {
	return len(m) == 0
}

func (m TransientMap) Exists(key string) bool {
	_, ok := m[key]
	return ok
}

func (m TransientMap) SetState(key string, state interface{}) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	m[key] = raw

	return nil
}

func (m TransientMap) GetState(key string, state interface{}) error {
	value, ok := m[key]
	if !ok {
		return errors.Errorf("transient map key [%s] does not exists", key)
	}
	if len(value) == 0 {
		return errors.Errorf("transient map key [%s] is empty", key)
	}

	return json.Unmarshal(value, state)
}

type TxID struct {
	Nonce   []byte
	Creator []byte
}

func (t *TxID) String() string {
	return fmt.Sprintf("[%s:%s]", base64.StdEncoding.EncodeToString(t.Nonce), base64.StdEncoding.EncodeToString(t.Creator))
}

type Envelope struct {
	e driver.Envelope
}

func (e *Envelope) Bytes() ([]byte, error) {
	return e.e.Bytes()
}

func (e *Envelope) FromBytes(raw []byte) error {
	return e.e.FromBytes(raw)
}

func (e *Envelope) Results() []byte {
	return e.e.Results()
}

func (e *Envelope) TxID() string {
	return e.e.TxID()
}

func (e *Envelope) Nonce() []byte {
	return e.e.Nonce()
}

func (e *Envelope) Creator() []byte {
	return e.e.Creator()
}

func (e *Envelope) MarshalJSON() ([]byte, error) {
	raw, err := e.e.Bytes()
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

func (e *Envelope) UnmarshalJSON(raw []byte) error {
	var r []byte
	err := json.Unmarshal(raw, &r)
	if err != nil {
		return err
	}
	return e.e.FromBytes(r)
}

type LoadedTransaction struct {
	loadedDataTx driver.LoadedDataTx
}

func (t *LoadedTransaction) ID() string {
	return t.loadedDataTx.ID()
}

func (t *LoadedTransaction) Commit() error {
	return t.loadedDataTx.Commit()
}

func (t *LoadedTransaction) CoSignAndClose() ([]byte, error) {
	return t.loadedDataTx.CoSignAndClose()
}

func (t *LoadedTransaction) Reads() map[string][]*types.DataRead {
	return t.loadedDataTx.Reads()
}

func (t *LoadedTransaction) Writes() map[string][]*types.DataWrite {
	return t.loadedDataTx.Writes()
}

func (t *LoadedTransaction) MustSignUsers() []string {
	return t.loadedDataTx.MustSignUsers()
}

func (t *LoadedTransaction) SignedUsers() []string {
	return t.loadedDataTx.SignedUsers()
}

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

func (d *Transaction) SignAndClose() ([]byte, error) {
	return d.dataTx.SignAndClose()
}

func (d *Transaction) Commit(sync bool) (string, *types.TxReceiptResponseEnvelope, error) {
	return d.dataTx.Commit(sync)
}

func (d *Transaction) AddMustSignUser(userID string) {
	d.dataTx.AddMustSignUser(userID)
}

type TransactionManager struct {
	ons *NetworkService
}

func (t *TransactionManager) ComputeTxID(id *TxID) string {
	txID := &driver.TxID{
		Nonce: id.Nonce, Creator: id.Creator,
	}
	res := t.ons.ons.TransactionManager().ComputeTxID(txID)
	id.Nonce = txID.Nonce
	id.Creator = txID.Creator
	return res
}

func (t *TransactionManager) NewEnvelope() *Envelope {
	return &Envelope{e: t.ons.ons.TransactionManager().NewEnvelope()}
}

func (t *TransactionManager) NewTransaction(txID string, creator string) (*Transaction, error) {
	session, err := t.ons.ons.SessionManager().NewSession(creator)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create session for txID [%s]", txID)
	}
	dataTx, err := session.DataTx(txID)
	if err != nil {
		return nil, err
	}
	return &Transaction{dataTx: dataTx}, nil
}

func (t *TransactionManager) NewLoadedTransaction(env []byte, creator string) (*LoadedTransaction, error) {
	session, err := t.ons.ons.SessionManager().NewSession(creator)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create session for creator [%s]", creator)
	}
	var e types.DataTxEnvelope
	if err = proto.Unmarshal(env, &e); err != nil {
		return nil, errors.WithMessagef(err, "failed to unmarshal env")
	}
	loadedDataTx, err := session.LoadDataTx(&e)
	if err != nil {
		return nil, err
	}
	return &LoadedTransaction{loadedDataTx: loadedDataTx}, nil
}

func (t *TransactionManager) CommitEnvelope(session *Session, envelope *Envelope) error {
	return t.ons.ons.TransactionManager().CommitEnvelope(session.s, envelope.e)
}

type MetadataService struct {
	ms driver.MetadataService
}

func (m *MetadataService) Exists(txid string) bool {
	return m.ms.Exists(txid)
}

func (m *MetadataService) StoreTransient(txid string, transientMap TransientMap) error {
	return m.ms.StoreTransient(txid, driver.TransientMap(transientMap))
}

func (m *MetadataService) LoadTransient(txid string) (TransientMap, error) {
	res, err := m.ms.LoadTransient(txid)
	if err != nil {
		return nil, err
	}
	return TransientMap(res), nil
}

type EnvelopeService struct {
	es driver.EnvelopeService
}

func (e *EnvelopeService) Exists(txid string) bool {
	return e.es.Exists(txid)
}

func (e *EnvelopeService) StoreEnvelope(txid string, env []byte) error {
	return e.es.StoreEnvelope(txid, env)
}

func (e *EnvelopeService) LoadEnvelope(txid string) ([]byte, error) {
	return e.es.LoadEnvelope(txid)
}
