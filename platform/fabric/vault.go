/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fabric

import (
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
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
	rws api.RWSet
}

func (r *RWSet) IsValid() error {
	return r.rws.IsValid()
}

func (r *RWSet) Clear(ns string) error {
	return r.rws.Clear(ns)
}

func (r *RWSet) SetState(namespace string, key string, value []byte) error {
	return r.rws.SetState(namespace, key, value)
}

func (r *RWSet) GetState(namespace string, key string, opts ...GetStateOpt) ([]byte, error) {
	var o []api.GetStateOpt
	for _, opt := range opts {
		o = append(o, api.GetStateOpt(opt))
	}
	return r.rws.GetState(namespace, key, o...)
}

func (r *RWSet) DeleteState(namespace string, key string) error {
	return r.rws.DeleteState(namespace, key)
}

func (r *RWSet) GetStateMetadata(namespace, key string, opts ...GetStateOpt) (map[string][]byte, error) {
	var o []api.GetStateOpt
	for _, opt := range opts {
		o = append(o, api.GetStateOpt(opt))
	}
	return r.rws.GetStateMetadata(namespace, key, o...)
}

func (r *RWSet) SetStateMetadata(namespace, key string, metadata map[string][]byte) error {
	return r.rws.SetStateMetadata(namespace, key, metadata)
}

func (r *RWSet) GetReadKeyAt(ns string, i int) (string, error) {
	return r.rws.GetReadKeyAt(ns, i)
}

func (r *RWSet) GetReadAt(ns string, i int) (string, []byte, error) {
	return r.rws.GetReadAt(ns, i)
}

func (r *RWSet) GetWriteAt(ns string, i int) (string, []byte, error) {
	return r.rws.GetWriteAt(ns, i)
}

func (r *RWSet) NumReads(ns string) int {
	return r.rws.NumReads(ns)
}

func (r *RWSet) NumWrites(ns string) int {
	return r.rws.NumWrites(ns)
}

func (r *RWSet) Namespaces() []string {
	return r.rws.Namespaces()
}

func (r *RWSet) AppendRWSet(raw []byte, nss ...string) error {
	return r.rws.AppendRWSet(raw, nss...)
}

func (r *RWSet) Bytes() ([]byte, error) {
	return r.rws.Bytes()
}

func (r *RWSet) String() string {
	return r.rws.String()
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

func (r *RWSet) RWS() api.RWSet {
	return r.rws
}

type Read struct {
	Key          string
	Raw          []byte
	Block        uint64
	IndexInBlock int
}

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

	return &Read{
		Key:          read.Key,
		Raw:          read.Raw,
		Block:        read.Block,
		IndexInBlock: read.IndexInBlock,
	}, nil
}

// Close releases resources occupied by the iterator
func (r *ResultsIterator) Close() {
	r.ri.Close()
}

type QueryExecutor struct {
	qe api.QueryExecutor
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
	api.TxidIterator
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

type Vault struct {
	ch api.Channel
}

func (c *Vault) GetLastTxID() (string, error) {
	return c.ch.GetLastTxID()
}

func (c *Vault) TxIDIterator(pos interface{}) (*TxIDIterator, error) {
	var iPos interface{}
	switch p := pos.(type) {
	case *api.SeekStart:
		iPos = &api.SeekStart{}
	case *api.SeekEnd:
		iPos = &api.SeekEnd{}
	case *api.SeekPos:
		iPos = &api.SeekPos{Txid: p.Txid}
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
	return c.ch.CommitTX(txid, block, indexInBloc)
}

func (c *Vault) NewQueryExecutor() (*QueryExecutor, error) {
	qe, err := c.ch.NewQueryExecutor()
	if err != nil {
		return nil, err
	}
	return &QueryExecutor{qe: qe}, nil
}

func (c *Vault) NewRWSet(txid string) (*RWSet, error) {
	rws, err := c.ch.NewRWSet(txid)
	if err != nil {
		return nil, err
	}
	return &RWSet{rws: rws}, nil
}

func (c *Vault) GetRWSet(txid string, rwset []byte) (*RWSet, error) {
	rws, err := c.ch.GetRWSet(txid, rwset)
	if err != nil {
		return nil, err
	}
	return &RWSet{rws: rws}, nil
}

func (c *Vault) GetEphemeralRWSet(rwset []byte) (*RWSet, error) {
	rws, err := c.ch.GetEphemeralRWSet(rwset)
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
	return c.ch.MetadataService().StoreTransient(id, api.TransientMap(tm))
}
