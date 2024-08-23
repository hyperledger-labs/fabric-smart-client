/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
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

type RWSet struct {
	driver.RWSet
}

func NewRWSet(rws driver.RWSet) *RWSet {
	return &RWSet{RWSet: rws}
}

// KeyExist returns true if a key exist in the rwset otherwise false.
func (r *RWSet) KeyExist(key string, ns string) (bool, error) {
	for i := 0; i < r.NumReads(ns); i++ {
		keyRead, _, err := r.GetReadAt(ns, i)
		if err != nil {
			return false, errors.WithMessagef(err, "Error reading key at [%d]", i)
		}
		if strings.Contains(keyRead, key) {
			return true, nil
		}
	}
	return false, nil
}

func (r *RWSet) Equals(rws interface{}, nss ...string) error {
	if rw, ok := rws.(*RWSet); !ok {
		return errors.Errorf("expected instance of *RWSet, got [%t]", rws)
	} else {
		return r.RWSet.Equals(rw.RWSet, nss...)
	}
}

type (
	Read            = vault.VersionedRead
	ResultsIterator = vault.VersionedResultsIterator
	ValidationCode  = fdriver.ValidationCode
	TxIDEntry       = driver.ByNum[ValidationCode]
	TxIDIterator    = fdriver.TxIDIterator
)

// Vault models a key-value store that can be updated by committing rwsets
type Vault struct {
	vault              fdriver.Vault
	txIDStore          fdriver.TXIDStore
	committer          fdriver.Committer
	transactionService fdriver.EndorserTransactionService
	envelopeService    fdriver.EnvelopeService
	metadataService    fdriver.MetadataService
}

func newVault(ch fdriver.Channel) *Vault {
	return &Vault{
		vault:              ch.Vault(),
		txIDStore:          ch.TXIDStore(),
		committer:          ch.Committer(),
		transactionService: ch.TransactionService(),
		envelopeService:    ch.EnvelopeService(),
		metadataService:    ch.MetadataService(),
	}
}

func (c *Vault) NewQueryExecutor() (driver.QueryExecutor, error) {
	return c.vault.NewQueryExecutor()
}

func (c *Vault) Status(id string) (ValidationCode, string, error) {
	return c.vault.Status(id)
}

func (c *Vault) GetLastTxID() (string, error) {
	return c.txIDStore.GetLastTxID()
}

// NewRWSet returns a RWSet for this ledger.
// A client may obtain more than one such simulator; they are made unique
// by way of the supplied txid
func (c *Vault) NewRWSet(txid string) (*RWSet, error) {
	rws, err := c.vault.NewRWSet(txid)
	if err != nil {
		return nil, err
	}
	return &RWSet{RWSet: rws}, nil
}

// GetRWSet returns a RWSet for this ledger whose content is unmarshalled
// from the passed bytes.
// A client may obtain more than one such simulator; they are made unique
// by way of the supplied txid
func (c *Vault) GetRWSet(txid string, rwset []byte) (*RWSet, error) {
	rws, err := c.vault.GetRWSet(txid, rwset)
	if err != nil {
		return nil, err
	}
	return &RWSet{RWSet: rws}, nil
}

// InspectRWSet returns an ephemeral RWSet for this ledger whose content is unmarshalled
// from the passed bytes.
// If namespaces is not empty, the returned RWSet will be filtered by the passed namespaces
func (c *Vault) InspectRWSet(rwset []byte, namespaces ...string) (*RWSet, error) {
	rws, err := c.vault.InspectRWSet(rwset, namespaces...)
	if err != nil {
		return nil, err
	}
	return &RWSet{RWSet: rws}, nil
}

func (c *Vault) StoreEnvelope(id string, env []byte) error {
	return c.envelopeService.StoreEnvelope(id, env)
}

func (c *Vault) StoreTransaction(id string, raw []byte) error {
	return c.transactionService.StoreTransaction(id, raw)
}

func (c *Vault) StoreTransient(id string, tm TransientMap) error {
	return c.metadataService.StoreTransient(id, fdriver.TransientMap(tm))
}

// DiscardTx discards the transaction with the given transaction id.
// If no error occurs, invoking Status on the same transaction id will return the Invalid flag.
func (c *Vault) DiscardTx(txID string, message string) error {
	return c.committer.DiscardTx(txID, message)
}

func (c *Vault) CommitTX(txID string, block driver.BlockNum, indexInBlock driver.TxNum) error {
	return c.committer.CommitTX(context.Background(), txID, block, indexInBlock, nil)
}
