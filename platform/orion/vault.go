/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"

	"github.com/pkg/errors"
)

type GetStateOpt int

const (
	FromStorage GetStateOpt = iota
	FromIntermediate
	FromBoth
)

type RWSet struct {
	rws driver.RWSet
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
	var o []driver.GetStateOpt
	for _, opt := range opts {
		o = append(o, driver.GetStateOpt(opt))
	}
	return r.rws.GetState(namespace, key, o...)
}

// DeleteState deletes the given namespace and key
func (r *RWSet) DeleteState(namespace string, key string) error {
	return r.rws.DeleteState(namespace, key)
}

func (r *RWSet) GetStateMetadata(namespace, key string, opts ...GetStateOpt) (map[string][]byte, error) {
	var o []driver.GetStateOpt
	for _, opt := range opts {
		o = append(o, driver.GetStateOpt(opt))
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

func (r *RWSet) RWS() driver.RWSet {
	return r.rws
}

type Read struct {
	Key          string
	Raw          []byte
	Block        uint64
	IndexInBlock int
}

// ResultsIterator models an query result iterator
type ResultsIterator struct {
	ri driver2.VersionedResultsIterator
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
	qe driver.QueryExecutor
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

// Vault models a key-value store that can be updated by committing rwsets
type Vault struct {
	v driver.Vault
}

// GetLastTxID returns the last transaction id committed
func (v *Vault) GetLastTxID() (string, error) {
	return v.v.GetLastTxID()
}

func (v *Vault) NewQueryExecutor() (*QueryExecutor, error) {
	qe, err := v.v.NewQueryExecutor()
	if err != nil {
		return nil, err
	}

	return &QueryExecutor{qe: qe}, nil
}

func (v *Vault) NewRWSet(txid string) (*RWSet, error) {
	rws, err := v.v.NewRWSet(txid)
	if err != nil {
		return nil, err
	}

	return &RWSet{rws: rws}, nil

}

func (v *Vault) GetRWSet(id string, results []byte) (*RWSet, error) {
	rws, err := v.v.GetRWSet(id, results)
	if err != nil {
		return nil, err
	}

	return &RWSet{rws: rws}, nil
}
