/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"encoding/json"
	"strings"

	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
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

type GetStateOpt int

const (
	FromStorage GetStateOpt = iota
	FromIntermediate
	FromBoth
)

type RWSet struct {
	rws fdriver.RWSet
}

func (r *RWSet) IsValid() error {
	return r.rws.IsValid()
}

func (r *RWSet) Clear(ns string) error {
	return r.rws.Clear(ns)
}

// SetState sets the given value for the given namespace and key.
func (r *RWSet) SetState(namespace string, key string, value []byte) error {
	return r.rws.SetState(namespace, key, value)
}

func (r *RWSet) GetState(namespace string, key string, opts ...GetStateOpt) ([]byte, error) {
	var o []fdriver.GetStateOpt
	for _, opt := range opts {
		o = append(o, fdriver.GetStateOpt(opt))
	}
	return r.rws.GetState(namespace, key, o...)
}

// DeleteState deletes the given namespace and key
func (r *RWSet) DeleteState(namespace string, key string) error {
	return r.rws.DeleteState(namespace, key)
}

func (r *RWSet) GetStateMetadata(namespace, key string, opts ...GetStateOpt) (map[string][]byte, error) {
	var o []fdriver.GetStateOpt
	for _, opt := range opts {
		o = append(o, fdriver.GetStateOpt(opt))
	}
	return r.rws.GetStateMetadata(namespace, key, o...)
}

// SetStateMetadata sets the metadata associated with an existing key-tuple <namespace, key>
func (r *RWSet) SetStateMetadata(namespace, key string, metadata map[string][]byte) error {
	return r.rws.SetStateMetadata(namespace, key, metadata)
}

func (r *RWSet) GetReadKeyAt(ns string, i int) (string, error) {
	return r.rws.GetReadKeyAt(ns, i)
}

// GetReadAt returns the i-th read (key, value) in the namespace ns  of this rwset.
// The value is loaded from the ledger, if present. If the key's version in the ledger
// does not match the key's version in the read, then it returns an error.
func (r *RWSet) GetReadAt(ns string, i int) (string, []byte, error) {
	return r.rws.GetReadAt(ns, i)
}

// GetWriteAt returns the i-th write (key, value) in the namespace ns of this rwset.
func (r *RWSet) GetWriteAt(ns string, i int) (string, []byte, error) {
	return r.rws.GetWriteAt(ns, i)
}

// NumReads returns the number of reads in the namespace ns  of this rwset.
func (r *RWSet) NumReads(ns string) int {
	return r.rws.NumReads(ns)
}

// NumWrites returns the number of writes in the namespace ns of this rwset.
func (r *RWSet) NumWrites(ns string) int {
	return r.rws.NumWrites(ns)
}

// Namespaces returns the namespace labels in this rwset.
func (r *RWSet) Namespaces() []string {
	return r.rws.Namespaces()
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

func (r *RWSet) AppendRWSet(raw []byte, nss ...string) error {
	return r.rws.AppendRWSet(raw, nss...)
}

func (r *RWSet) Bytes() ([]byte, error) {
	return r.rws.Bytes()
}

func (r *RWSet) Done() {
	r.rws.Done()
}

func (r *RWSet) Equals(rws interface{}, nss ...string) error {
	r, ok := rws.(*RWSet)
	if !ok {
		return errors.Errorf("expected instance of *RWSet, got [%t]", rws)
	}
	return r.rws.Equals(r.rws, nss...)
}

func (r *RWSet) RWS() fdriver.RWSet {
	return r.rws
}

type Read driver.VersionedRead

func (v *Read) K() string {
	return v.Key
}

func (v *Read) V() []byte {
	return v.Raw
}

// ResultsIterator models an query result iterator
type ResultsIterator struct {
	ri driver.VersionedResultsIterator
}

// Next returns the next item in the result set. The `QueryResult` is expected to be nil when
// the iterator gets exhausted
func (r *ResultsIterator) Next() (*Read, error) {
	read, err := r.ri.Next()
	if err != nil {
		return nil, err
	}
	if read == nil {
		return nil, nil
	}

	return (*Read)(read), nil
}

// Close releases resources occupied by the iterator
func (r *ResultsIterator) Close() {
	r.ri.Close()
}

type QueryExecutor struct {
	qe fdriver.QueryExecutor
}

func (qe *QueryExecutor) GetState(namespace string, key string) ([]byte, error) {
	return qe.qe.GetState(namespace, key)
}

func (qe *QueryExecutor) GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error) {
	return qe.qe.GetStateMetadata(namespace, key)
}

func (qe *QueryExecutor) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (*ResultsIterator, error) {
	ri, err := qe.qe.GetStateRangeScanIterator(namespace, startKey, endKey)
	if err != nil {
		return nil, err
	}
	return &ResultsIterator{ri: ri}, nil
}

func (qe *QueryExecutor) Done() {
	qe.qe.Done()
}

type ValidationCode int

const (
	_               ValidationCode = iota
	Valid                          // Transaction is valid and committed
	Invalid                        // Transaction is invalid and has been discarded
	Busy                           // Transaction does not yet have a validity state
	Unknown                        // Transaction is unknown
	HasDependencies                // Transaction is unknown but has known dependencies
)

type SeekStart struct{}

type SeekEnd struct{}

type SeekPos struct {
	Txid string
}

type TxIDEntry struct {
	Txid string
	Code ValidationCode
}

type TxIDIterator struct {
	fdriver.TxidIterator
}

func (t *TxIDIterator) Next() (*TxIDEntry, error) {
	n, err := t.TxidIterator.Next()
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, nil
	}
	return &TxIDEntry{
		Txid: n.Txid,
		Code: ValidationCode(n.Code),
	}, nil
}

func (t *TxIDIterator) Close() {
	t.TxidIterator.Close()
}

// Vault models a key-value store that can be updated by committing rwsets
type Vault struct {
	ch fdriver.Channel
}

// GetLastTxID returns the last transaction id committed
func (c *Vault) GetLastTxID() (string, error) {
	return c.ch.GetLastTxID()
}

func (c *Vault) TxIDIterator(pos interface{}) (*TxIDIterator, error) {
	var iPos interface{}
	switch p := pos.(type) {
	case *fdriver.SeekStart:
		iPos = &fdriver.SeekStart{}
	case *fdriver.SeekEnd:
		iPos = &fdriver.SeekEnd{}
	case *fdriver.SeekPos:
		iPos = &fdriver.SeekPos{Txid: p.Txid}
	default:
		return nil, errors.Errorf("invalid position %T", pos)
	}
	it, err := c.ch.Iterator(iPos)
	if err != nil {
		return nil, err
	}
	return &TxIDIterator{TxidIterator: it}, nil
}

func (c *Vault) Status(txid string) (ValidationCode, []string, error) {
	code, deps, err := c.ch.Status(txid)
	if err != nil {
		return Unknown, deps, err
	}
	return ValidationCode(code), deps, nil
}

func (c *Vault) DiscardTx(txid string) error {
	return c.ch.DiscardTx(txid)
}

func (c *Vault) CommitTX(txid string, block uint64, indexInBloc int) error {
	return c.ch.CommitTX(txid, block, indexInBloc, nil)
}

// NewQueryExecutor gives handle to a query executor.
// A client can obtain more than one 'QueryExecutor's for parallel execution.
// Any synchronization should be performed at the implementation level if required
func (c *Vault) NewQueryExecutor() (*QueryExecutor, error) {
	qe, err := c.ch.NewQueryExecutor()
	if err != nil {
		return nil, err
	}
	return &QueryExecutor{qe: qe}, nil
}

// NewRWSet returns a RWSet for this ledger.
// A client may obtain more than one such simulator; they are made unique
// by way of the supplied txid
func (c *Vault) NewRWSet(txid string) (*RWSet, error) {
	rws, err := c.ch.NewRWSet(txid)
	if err != nil {
		return nil, err
	}
	return &RWSet{rws: rws}, nil
}

// GetRWSet returns a RWSet for this ledger whose content is unmarshalled
// from the passed bytes.
// A client may obtain more than one such simulator; they are made unique
// by way of the supplied txid
func (c *Vault) GetRWSet(txid string, rwset []byte) (*RWSet, error) {
	rws, err := c.ch.GetRWSet(txid, rwset)
	if err != nil {
		return nil, err
	}
	return &RWSet{rws: rws}, nil
}

// GetEphemeralRWSet returns an ephemeral RWSet for this ledger whose content is unmarshalled
// from the passed bytes.
// If namespaces is not empty, the returned RWSet will be filtered by the passed namespaces
func (c *Vault) GetEphemeralRWSet(rwset []byte, namespaces ...string) (*RWSet, error) {
	rws, err := c.ch.GetEphemeralRWSet(rwset, namespaces...)
	if err != nil {
		return nil, err
	}
	return &RWSet{rws: rws}, nil
}

func (c *Vault) StoreEnvelope(id string, env []byte) error {
	return c.ch.EnvelopeService().StoreEnvelope(id, env)
}

func (c *Vault) StoreTransaction(id string, raw []byte) error {
	return c.ch.TransactionService().StoreTransaction(id, raw)
}

func (c *Vault) StoreTransient(id string, tm TransientMap) error {
	return c.ch.MetadataService().StoreTransient(id, fdriver.TransientMap(tm))
}
