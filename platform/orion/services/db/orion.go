/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package db

import (
	"encoding/base64"
	"strings"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	dbproto "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/keys"
	"github.com/pkg/errors"
)

type cacheValue struct {
	v *driver.VersionedRead
	k []byte
}

type cache interface {
	Get(key string) (interface{}, bool)
	Add(key string, value interface{})
	Delete(key string)
}

type ItemList struct {
	items []cacheValue
	sync.RWMutex
}

func (i *ItemList) Get(index int) (*driver.VersionedRead, bool) {
	i.RLock()
	defer i.RUnlock()
	if index < 0 || index >= len(i.items) {
		return nil, false
	}
	return i.items[index].v, true
}

func (i *ItemList) Set(index int, v *driver.VersionedRead, k []byte) {
	i.Lock()
	defer i.Unlock()

	// if not in capacity, then skip
	if index < 0 || index+1 > cap(i.items) {
		return
	}

	// if not in length, then append
	if index >= len(i.items) {
		i.items = i.items[:index+1]
	}

	i.items[index].v = v
	i.items[index].k = k
}

func (i *ItemList) GetLast() []byte {
	i.RLock()
	defer i.RUnlock()
	if len(i.items) == 0 {
		return nil
	}
	return i.items[len(i.items)-1].k
}

type OrionBackend interface {
	SessionManager() *orion.SessionManager
	TransactionManager() *orion.TransactionManager
}

type Orion struct {
	name string

	ons       OrionBackend
	txManager *orion.TransactionManager
	creator   string
	txn       *orion.Transaction
	deletes   []string
	txnLock   sync.RWMutex
	cache     cache

	itemsMap map[string]*ItemList
}

func (db *Orion) SetState(namespace, key string, value []byte, block, txnum uint64) error {
	if db.txn == nil {
		panic("programming error, writing without ongoing update")
	}

	dbKey := dbKey(namespace, key)

	v, err := db.versionedValue(db.txn, dbKey)
	if err != nil {
		return err
	}

	v.Value = value
	v.Block = block
	v.Txnum = txnum

	bytes, err := proto.Marshal(v)
	if err != nil {
		return errors.Wrapf(err, "could not marshal VersionedValue for key %s", dbKey)
	}

	err = db.txn.Put(db.name, dbKey, bytes, nil)
	if err != nil {
		return errors.Wrapf(err, "could not set value for key %s", dbKey)
	}

	return nil
}

func (db *Orion) GetState(namespace, key string) ([]byte, uint64, uint64, error) {
	dbKey := dbKey(namespace, key)

	txn, err := db.txManager.NewTransaction("", db.creator)
	if err != nil {
		return nil, 0, 0, err
	}
	v, err := db.versionedValue(txn, dbKey)
	if err != nil {
		return nil, 0, 0, err
	}
	return v.Value, v.Block, v.Txnum, nil
}

func (db *Orion) DeleteState(namespace, key string) error {
	if db.txn == nil {
		panic("programming error, writing without ongoing update")
	}

	dbKey := dbKey(namespace, key)

	err := db.txn.Delete(db.name, dbKey)
	if err != nil {
		return errors.Wrapf(err, "could not delete value for key %s", dbKey)
	}
	db.deletes = append(db.deletes, dbKey)

	return nil
}

func (db *Orion) GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error) {
	dbKey := dbKey(namespace, key)

	txn, err := db.txManager.NewTransaction("", db.creator)
	if err != nil {
		return nil, 0, 0, err
	}

	v, err := db.versionedValue(txn, dbKey)
	if err != nil {
		return nil, 0, 0, err
	}

	return v.Meta, v.Block, v.Txnum, nil
}

func (db *Orion) SetStateMetadata(namespace, key string, metadata map[string][]byte, block, txnum uint64) error {
	if db.txn == nil {
		panic("programming error, writing without ongoing update")
	}

	dbKey := dbKey(namespace, key)

	v, err := db.versionedValue(db.txn, dbKey)
	if err != nil {
		return err
	}

	v.Meta = metadata
	v.Block = block
	v.Txnum = txnum

	bytes, err := proto.Marshal(v)
	if err != nil {
		return errors.Wrapf(err, "could not marshal VersionedValue for key %s", dbKey)
	}

	err = db.txn.Put(db.name, dbKey, bytes, nil)
	if err != nil {
		return errors.Wrapf(err, "could not set value for key %s", dbKey)
	}

	return nil
}

func (db *Orion) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (driver.VersionedResultsIterator, error) {
	s, err := db.ons.SessionManager().NewSession(db.creator)
	if err != nil {
		return nil, errors.WithMessagef(err, "could not create session")
	}
	qe, err := s.QueryExecutor(db.name)
	if err != nil {
		return nil, errors.WithMessagef(err, "could not create query executor for %s", db.name)
	}

	sk := dbKey(namespace, startKey)
	ek := dbKey(namespace, endKey)

	it, err := qe.GetDataByRange(sk, ek, 100)
	if err != nil {
		return nil, errors.WithMessagef(err, "could not get data by range for %s", db.name)
	}
	return &VersionedResultsIterator{it: it}, nil
}

func (db *Orion) GetCachedStateRangeScanIterator(namespace string, startKey string, endKey string) (driver.VersionedResultsIterator, error) {
	return db.GetStateRangeScanIterator(namespace, startKey, endKey)
}

func (db *Orion) Close() error {
	// TODO: what to do with db.txn if it's not nil?
	return nil
}

func (db *Orion) BeginUpdate() error {
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.txn != nil {
		return errors.New("previous commit in progress")
	}

	txn, err := db.txManager.NewTransaction("", db.creator)
	if err != nil {
		return errors.Wrapf(err, "could not begin transaction")
	}
	db.txn = txn
	db.deletes = nil

	return nil
}

func (db *Orion) Commit() error {
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.txn == nil {
		return errors.New("no commit in progress")
	}

	_, _, err := db.txn.Commit(true)
	if err != nil {
		return errors.Wrap(err, "could not commit transaction")
	}

	// delete from the cache the deleted keys
	for _, key := range db.deletes {
		db.cache.Delete(key)
	}

	db.txn = nil
	db.deletes = nil

	db.itemsMap = map[string]*ItemList{}

	return nil
}

func (db *Orion) Discard() error {
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.txn == nil {
		return errors.New("no commit in progress")
	}

	// no discard function to be called on txn

	db.txn = nil
	db.deletes = nil

	return nil
}

func dbKey(namespace, key string) string {
	k := orionKey(namespace + keys.NamespaceSeparator + key)
	components := strings.Split(k, "~")
	var b strings.Builder
	for _, component := range components {
		b.WriteString(base64.StdEncoding.EncodeToString([]byte(component)))
		b.WriteString("~")
	}
	return b.String()
}

func orionKey(key string) string {
	return strings.ReplaceAll(key, string(rune(0)), "~")
}

func (db *Orion) versionedValue(txn *orion.Transaction, dbKey string) (*dbproto.VersionedValue, error) {
	v, _, err := txn.Get(db.name, dbKey)
	if err != nil {
		return nil, errors.Wrapf(err, "could not retrieve item for key %s", dbKey)
	}
	if len(v) == 0 {
		return &dbproto.VersionedValue{
			Version: dbproto.V1,
		}, nil
	}

	return versionedValue(v, dbKey)
}

func versionedValue(v []byte, dbKey string) (*dbproto.VersionedValue, error) {
	protoValue := &dbproto.VersionedValue{}
	if err := proto.Unmarshal(v, protoValue); err != nil {
		return nil, errors.Wrapf(err, "could not unmarshal VersionedValue for key %s", dbKey)
	}
	if protoValue.Version != dbproto.V1 {
		return nil, errors.Errorf("invalid version, expected %d, got %d", dbproto.V1, protoValue.Version)
	}
	return protoValue, nil
}

type VersionedResultsIterator struct {
	it *orion.QueryIterator
}

func (v *VersionedResultsIterator) Next() (*driver.VersionedRead, error) {
	r, _, err := v.it.Next()
	if err != nil {
		return nil, errors.Wrap(err, "could not get next item")
	}
	if r == nil {
		return nil, nil
	}
	return &driver.VersionedRead{
		Key:          r.Key,
		Raw:          r.Value,
		Block:        r.Metadata.Version.BlockNum,
		IndexInBlock: int(r.Metadata.Version.TxNum),
	}, nil
}

func (v *VersionedResultsIterator) Close() {
	// nothing to do
}
