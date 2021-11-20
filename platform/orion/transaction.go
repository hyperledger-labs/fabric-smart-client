/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
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
