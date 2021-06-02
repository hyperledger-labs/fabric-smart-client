/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package mem

import (
	"errors"
	"sort"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("view-sdk")

type versionedValue struct {
	block    uint64
	txnum    uint64
	value    []byte
	metadata map[string][]byte
}

type db struct {
	keys  map[string]map[string]*versionedValue
	mutex sync.Mutex
	txn   map[string]map[string]*versionedValue
}

type rangeIterator struct {
	beg  int
	cur  int
	end  int
	keys []string
	db   *db
	ns   string
}

func (r *rangeIterator) Next() (*driver.VersionedRead, error) {
	if r.cur == r.end {
		return nil, nil
	}

	var err error
	var idx uint64
	kv := &driver.VersionedRead{Key: r.keys[r.cur]}
	kv.Raw, kv.Block, idx, err = r.db.GetState(r.ns, r.keys[r.cur])
	kv.IndexInBlock = int(idx)
	if err != nil {
		return nil, err
	}

	r.cur++

	return kv, nil
}

func (r *rangeIterator) Close() {}

func New() *db {
	return &db{
		keys:  map[string]map[string]*versionedValue{},
		mutex: sync.Mutex{},
	}
}

func (db *db) Close() error {
	return nil
}

func (db *db) BeginUpdate() error {
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

func (db *db) Commit() error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.txn == nil {
		return errors.New("no commit in progress")
	}

	db.keys = db.txn
	db.txn = nil

	return nil
}

func (db *db) Discard() error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.txn == nil {
		return errors.New("no commit in progress")
	}

	db.txn = nil

	return nil
}

func (db *db) mapForNamespaceForReading(ns string, add bool) map[string]*versionedValue {
	return db.mapForNamespace(ns, add, db.keys)
}

func (db *db) mapForNamespaceForWriting(ns string, add bool) map[string]*versionedValue {
	return db.mapForNamespace(ns, add, db.txn)
}

func (db *db) mapForNamespace(ns string, add bool, mm map[string]map[string]*versionedValue) map[string]*versionedValue {
	m, in := mm[ns]
	if !in && add {
		m = map[string]*versionedValue{}
		mm[ns] = m
	}

	return m
}

func (db *db) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (driver.VersionedResultsIterator, error) {
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

func (db *db) GetState(namespace string, key string) ([]byte, uint64, uint64, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	vv, in := db.mapForNamespaceForReading(namespace, false)[key]
	if !in {
		return nil, 0, 0, nil
	}

	return append([]byte(nil), vv.value...), vv.block, vv.txnum, nil
}

func (db *db) GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

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

func (db *db) SetState(namespace string, key string, value []byte, block, txnum uint64) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.txn == nil {
		panic("programming error, writing without ongoing update")
	}

	logger.Debugf("Set stat [%s,%s]", namespace, key)

	vv, in := db.mapForNamespaceForWriting(namespace, true)[key]
	if !in {
		vv = &versionedValue{}
		db.mapForNamespaceForWriting(namespace, true)[key] = vv
	}

	vv.block = block
	vv.txnum = txnum
	vv.value = append([]byte(nil), value...)

	return nil
}

func (db *db) SetStateMetadata(namespace, key string, metadata map[string][]byte, block, txnum uint64) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.txn == nil {
		panic("programming error, writing without ongoing update")
	}

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

func (db *db) DeleteState(namespace string, key string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if db.txn == nil {
		panic("programming error, writing without ongoing update")
	}

	nsm := db.mapForNamespaceForWriting(namespace, false)
	delete(nsm, key)
	if len(nsm) == 0 {
		delete(db.txn, namespace)
	}

	return nil
}
