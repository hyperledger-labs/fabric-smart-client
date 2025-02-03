/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"encoding/json"
	"strings"

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

func (m TransientMap) GetState(key driver.PKey, state interface{}) error {
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

type lastTxGetter interface {
	GetLast(ctx context.Context) (*driver.TxStatus, error)
}

// Vault models a key-value store that can be updated by committing rwsets
type Vault struct {
	vault              fdriver.Vault
	vaultStore         lastTxGetter
	committer          fdriver.Committer
	transactionService fdriver.EndorserTransactionService
	envelopeService    fdriver.EnvelopeService
	metadataService    fdriver.MetadataService
}

func newVault(ch fdriver.Channel) *Vault {
	return &Vault{
		vault:              ch.Vault(),
		committer:          ch.Committer(),
		transactionService: ch.TransactionService(),
		envelopeService:    ch.EnvelopeService(),
		metadataService:    ch.MetadataService(),
		vaultStore:         ch.VaultStore(),
	}
}

func (c *Vault) NewQueryExecutor(ctx context.Context) (driver.QueryExecutor, error) {
	return c.vault.NewQueryExecutor(ctx)
}

func (c *Vault) Status(ctx context.Context, id driver.TxID) (fdriver.ValidationCode, string, error) {
	return c.vault.Status(ctx, id)
}

func (c *Vault) GetLastTxID(ctx context.Context) (string, error) {
	last, err := c.vaultStore.GetLast(ctx)
	if err != nil {
		return "", err
	}
	return last.TxID, nil
}

// NewRWSet returns a RWSet for this ledger.
// A client may obtain more than one such simulator; they are made unique
// by way of the supplied txid
func (c *Vault) NewRWSet(ctx context.Context, txID driver.TxID) (*RWSet, error) {
	rws, err := c.vault.NewRWSet(ctx, txID)
	if err != nil {
		return nil, err
	}
	return &RWSet{RWSet: rws}, nil
}

// GetRWSet returns a RWSet for this ledger whose content is unmarshalled
// from the passed bytes.
// A client may obtain more than one such simulator; they are made unique
// by way of the supplied txid
func (c *Vault) GetRWSet(ctx context.Context, txid driver.TxID, rwset []byte) (*RWSet, error) {
	rws, err := c.vault.GetRWSet(ctx, txid, rwset)
	if err != nil {
		return nil, err
	}
	return &RWSet{RWSet: rws}, nil
}

// InspectRWSet returns an ephemeral RWSet for this ledger whose content is unmarshalled
// from the passed bytes.
// If namespaces is not empty, the returned RWSet will be filtered by the passed namespaces
func (c *Vault) InspectRWSet(ctx context.Context, rwset []byte, namespaces ...driver.Namespace) (*RWSet, error) {
	rws, err := c.vault.InspectRWSet(ctx, rwset, namespaces...)
	if err != nil {
		return nil, err
	}
	return &RWSet{RWSet: rws}, nil
}

func (c *Vault) StoreEnvelope(ctx context.Context, id driver.TxID, env []byte) error {
	return c.envelopeService.StoreEnvelope(id, env)
}

func (c *Vault) StoreTransaction(ctx context.Context, id driver.TxID, raw []byte) error {
	return c.transactionService.StoreTransaction(id, raw)
}

func (c *Vault) StoreTransient(ctx context.Context, id driver.TxID, tm TransientMap) error {
	return c.metadataService.StoreTransient(id, fdriver.TransientMap(tm))
}

// DiscardTx discards the transaction with the given transaction id.
// If no error occurs, invoking Status on the same transaction id will return the Invalid flag.
func (c *Vault) DiscardTx(ctx context.Context, txID driver.TxID, message string) error {
	return c.committer.DiscardTx(ctx, txID, message)
}

func (c *Vault) CommitTX(ctx context.Context, txID driver.TxID, block driver.BlockNum, indexInBlock driver.TxNum) error {
	return c.committer.CommitTX(ctx, txID, block, indexInBlock, nil)
}
