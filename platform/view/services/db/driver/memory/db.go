/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"errors"
	"sort"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/keys"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"go.uber.org/zap/zapcore"
)

var logger = flogging.MustGetLogger("view-sdk")

type versionedValue struct {
	block    uint64
	txnum    uint64
	value    []byte
	metadata map[string][]byte
}

type database struct {
	keys  map[string]map[string]*versionedValue
	mutex sync.Mutex
	txn   map[string]map[string]*versionedValue
}

type rangeIterator struct {
	beg  int
	cur  int
	end  int
	keys []string
	db   *database
	ns   string
}

func (r *rangeIterator) Next() (*driver.VersionedRead, error) {
	if r.cur == r.end {
		return nil, nil
	}

	vv, err := r.db.GetState(r.ns, r.keys[r.cur])
	if err != nil {
		return nil, err
	}
	kv := &driver.VersionedRead{
		Key:   r.keys[r.cur],
		Raw:   vv.Raw,
		Block: vv.Block,
		TxNum: vv.TxNum,
	}

	r.cur++

	return kv, nil
}

func (r *rangeIterator) Close() {}

func New() *database {
	return &database{
		keys:  map[string]map[string]*versionedValue{},
		mutex: sync.Mutex{},
	}
}

func (db *database) Close() error {
	return nil
}

func (db *database) BeginUpdate() error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.txn != nil {
		return errors.New("previous commit in progress")
	}

	db.txn = map[string]map[string]*versionedValue{}
	for k := range db.keys {
		db.txn[k] = map[string]*versionedValue{}
		for kk, vv := range db.keys[k] {
			db.txn[k][kk] = &versionedValue{
				block: vv.block,
				txnum: vv.txnum,
			}

			if vv.value != nil {
				db.txn[k][kk].value = append([]byte{}, vv.value...)
			}

			if vv.metadata != nil {
				db.txn[k][kk].metadata = map[string][]byte{}
			}

			for kkk, vvv := range vv.metadata {
				if vvv != nil {
					db.txn[k][kk].metadata[kkk] = append([]byte{}, vvv...)
				}
			}
		}
	}

	return nil
}

func (db *database) Commit() error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.txn == nil {
		return errors.New("no commit in progress")
	}

	db.keys = db.txn
	db.txn = nil

	return nil
}

func (db *database) Discard() error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.txn == nil {
		return errors.New("no commit in progress")
	}

	db.txn = nil

	return nil
}

func (db *database) mapForNamespaceForReading(ns string, add bool) map[string]*versionedValue {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	return db.mapForNamespace(ns, add, db.keys)
}

func (db *database) mapForNamespaceForWriting(ns string, add bool) map[string]*versionedValue {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	return db.mapForNamespace(ns, add, db.txn)
}

func (db *database) mapForNamespace(ns string, add bool, mm map[string]map[string]*versionedValue) map[string]*versionedValue {
	m, in := mm[ns]
	if !in && add {
		m = map[string]*versionedValue{}
		mm[ns] = m
	}

	return m
}

func (db *database) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (driver.VersionedResultsIterator, error) {
	vv := db.mapForNamespaceForReading(namespace, false)
	sortedKeys := make([]string, 0, len(vv))
	for k := range vv {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	beg := sort.SearchStrings(sortedKeys, startKey)
	end := sort.SearchStrings(sortedKeys, endKey)

	if startKey == "" {
		beg = 0
	}
	if endKey == "" {
		end = len(sortedKeys)
	}

	return &rangeIterator{
		beg:  beg,
		cur:  beg,
		end:  end,
		ns:   namespace,
		db:   db,
		keys: sortedKeys,
	}, nil
}

func (db *database) GetStateSetIterator(ns string, ks ...string) (driver.VersionedResultsIterator, error) {
	reads := make([]*driver.VersionedRead, len(ks))
	for i, key := range ks {
		vv, err := db.GetState(ns, key)
		if err != nil {
			return nil, err
		}
		reads[i] = &driver.VersionedRead{
			Key:   key,
			Raw:   vv.Raw,
			Block: vv.Block,
			TxNum: vv.TxNum,
		}
	}
	return &keys.DummyVersionedIterator{Items: reads}, nil
}

func (db *database) GetCachedStateRangeScanIterator(namespace string, startKey string, endKey string) (driver.VersionedResultsIterator, error) {
	return db.GetStateRangeScanIterator(namespace, startKey, endKey)
}

func (db *database) GetState(namespace core.Namespace, key string) (driver.VersionedValue, error) {
	vv, in := db.mapForNamespaceForReading(namespace, false)[key]
	if !in {
		return driver.VersionedValue{}, nil
	}

	return driver.VersionedValue{Raw: append([]byte(nil), vv.value...), Block: vv.block, TxNum: vv.txnum}, nil
}

func (db *database) GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error) {
	vv, in := db.mapForNamespaceForReading(namespace, false)[key]
	if !in {
		return nil, 0, 0, nil
	}

	metadata := map[string][]byte{}
	for k, v := range vv.metadata {
		metadata[k] = append([]byte(nil), v...)
	}
	return metadata, vv.block, vv.txnum, nil
}

func (db *database) SetState(namespace core.Namespace, key string, value driver.VersionedValue) error {
	if len(value.Raw) == 0 {
		logger.Warnf("set key [%s:%d:%d] to nil value, will be deleted instead", key, value.Block, value.TxNum)
		return db.DeleteState(namespace, key)
	}

	db.mutex.Lock()
	if db.txn == nil {
		db.mutex.Unlock()
		panic("programming error, writing without ongoing update")
	}
	db.mutex.Unlock()

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Set stat [%s,%s]", namespace, key)
	}

	vv, in := db.mapForNamespaceForWriting(namespace, true)[key]
	if !in {
		vv = &versionedValue{}
		db.mapForNamespaceForWriting(namespace, true)[key] = vv
	}

	vv.block = value.Block
	vv.txnum = value.TxNum
	vv.value = append([]byte(nil), value.Raw...)

	return nil
}

func (db *database) SetStateMetadata(namespace, key string, metadata map[string][]byte, block, txnum uint64) error {
	db.mutex.Lock()
	if db.txn == nil {
		db.mutex.Unlock()
		panic("programming error, writing without ongoing update")
	}
	db.mutex.Unlock()

	vv, in := db.mapForNamespaceForWriting(namespace, true)[key]
	if !in {
		vv = &versionedValue{}
		db.mapForNamespaceForWriting(namespace, true)[key] = vv
	}

	vv.block = block
	vv.txnum = txnum
	vv.metadata = map[string][]byte{}
	for k, v := range metadata {
		vv.metadata[k] = append([]byte(nil), v...)
	}

	return nil
}

func (db *database) DeleteState(namespace string, key string) error {
	db.mutex.Lock()
	if db.txn == nil {
		db.mutex.Unlock()
		panic("programming error, writing without ongoing update")
	}
	db.mutex.Unlock()

	nsm := db.mapForNamespaceForWriting(namespace, false)
	delete(nsm, key)
	if len(nsm) == 0 {
		delete(db.txn, namespace)
	}

	return nil
}
