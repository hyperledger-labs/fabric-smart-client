/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badger

import (
	"bytes"
	"strings"
	"sync"

	"github.com/dgraph-io/badger/v3"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/cache/secondcache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	dbproto "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/keys"
)

var (
	cacheEmptyProtoValue = &dbproto.VersionedValue{}
)

type cache interface {
	Get(key string) (interface{}, bool)
	Add(key string, value interface{})
	Delete(key string)
}

type ItemList struct {
	items []*driver.VersionedRead
	sync.RWMutex
}

func (i *ItemList) Get(index int) (*driver.VersionedRead, bool) {
	i.RLock()
	defer i.RUnlock()
	if index < 0 || index >= len(i.items) {
		return nil, false
	}
	return i.items[index], true
}

func (i *ItemList) Set(index int, v *driver.VersionedRead) {
	i.Lock()
	defer i.Unlock()
	if index < 0 || index >= len(i.items) {
		return
	}
	i.items[index] = v
}

type badgerDB struct {
	db *badger.DB

	txn     *badger.Txn
	deletes []string
	txnLock sync.Mutex
	cache   cache

	itemsMap     map[string]*ItemList
	itemsMapLock sync.RWMutex
}

func OpenDB(path string) (*badgerDB, error) {
	if len(path) == 0 {
		return nil, errors.Errorf("path cannot be empty")
	}

	db, err := badger.Open(badger.DefaultOptions(path))
	if err != nil {
		return nil, errors.Wrapf(err, "could not open DB at '%s'", path)
	}

	return &badgerDB{
		db:       db,
		cache:    secondcache.New(20000),
		itemsMap: map[string]*ItemList{},
	}, nil
}

func (db *badgerDB) Close() error {

	// TODO: what to do with db.txn if it's not nil?

	err := db.db.Close()
	if err != nil {
		return errors.Wrap(err, "could not close DB")
	}

	return nil
}

func (db *badgerDB) BeginUpdate() error {
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.txn != nil {
		return errors.New("previous commit in progress")
	}

	db.txn = db.db.NewTransaction(true)
	db.deletes = nil

	return nil
}

func (db *badgerDB) Commit() error {
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.txn == nil {
		return errors.New("no commit in progress")
	}

	err := db.txn.Commit()
	if err != nil {
		return errors.Wrap(err, "could not commit transaction")
	}

	// delete from the cache the deleted keys
	for _, key := range db.deletes {
		db.cache.Delete(key)
	}

	db.txn = nil
	db.deletes = nil

	db.itemsMapLock.Lock()
	defer db.itemsMapLock.Unlock()
	db.itemsMap = map[string]*ItemList{}

	return nil
}

func (db *badgerDB) Discard() error {
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.txn == nil {
		return errors.New("no commit in progress")
	}

	db.txn.Discard()

	db.txn = nil
	db.deletes = nil

	return nil
}

func dbKey(namespace, key string) string {
	return namespace + keys.NamespaceSeparator + key
}

func (db *badgerDB) versionedValue(txn *badger.Txn, dbKey string) (*dbproto.VersionedValue, error) {
	it, err := txn.Get([]byte(dbKey))
	if err == badger.ErrKeyNotFound {
		return &dbproto.VersionedValue{
			Version: dbproto.V1,
		}, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "could not retrieve item for key %s", dbKey)
	}

	return versionedValue(it, dbKey)
}

func versionedValue(item *badger.Item, dbKey string) (*dbproto.VersionedValue, error) {
	protoValue := &dbproto.VersionedValue{}
	err := item.Value(func(val []byte) error {
		if err := proto.Unmarshal(val, protoValue); err != nil {
			return errors.Wrapf(err, "could not unmarshal VersionedValue for key %s", dbKey)
		}

		if protoValue.Version != dbproto.V1 {
			return errors.Errorf("invalid version, expected %d, got %d", dbproto.V1, protoValue.Version)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "could not get value for key %s", dbKey)
	}

	return protoValue, nil
}

func (db *badgerDB) SetState(namespace, key string, value []byte, block, txnum uint64) error {
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

	err = db.txn.Set([]byte(dbKey), bytes)
	if err != nil {
		return errors.Wrapf(err, "could not set value for key %s", dbKey)
	}

	return nil
}

func (db *badgerDB) SetStateMetadata(namespace, key string, metadata map[string][]byte, block, txnum uint64) error {
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

	err = db.txn.Set([]byte(dbKey), bytes)
	if err != nil {
		return errors.Wrapf(err, "could not set value for key %s", dbKey)
	}

	return nil
}

func (db *badgerDB) DeleteState(namespace, key string) error {
	if db.txn == nil {
		panic("programming error, writing without ongoing update")
	}

	dbKey := dbKey(namespace, key)

	err := db.txn.Delete([]byte(dbKey))
	if err != nil {
		return errors.Wrapf(err, "could not delete value for key %s", dbKey)
	}
	db.deletes = append(db.deletes, dbKey)

	return nil
}

func (db *badgerDB) GetState(namespace, key string) ([]byte, uint64, uint64, error) {
	dbKey := dbKey(namespace, key)

	txn := db.db.NewTransaction(false)
	defer txn.Discard()

	v, err := db.versionedValue(txn, dbKey)
	if err != nil {
		return nil, 0, 0, err
	}

	return v.Value, v.Block, v.Txnum, nil
}

func (db *badgerDB) GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error) {
	dbKey := dbKey(namespace, key)

	txn := db.db.NewTransaction(false)
	defer txn.Discard()

	v, err := db.versionedValue(txn, dbKey)
	if err != nil {
		return nil, 0, 0, err
	}

	return v.Meta, v.Block, v.Txnum, nil
}

type rangeScanIterator struct {
	txn       *badger.Txn
	it        *badger.Iterator
	startKey  string
	endKey    string
	namespace string
}

func (r *rangeScanIterator) Next() (*driver.VersionedRead, error) {
	if !r.it.Valid() {
		return nil, nil
	}

	item := r.it.Item()
	if r.endKey != "" && (bytes.Compare(item.Key(), []byte(dbKey(r.namespace, r.endKey))) >= 0) {
		return nil, nil
	}

	v, err := versionedValue(item, string(item.Key()))
	if err != nil {
		return nil, errors.Wrapf(err, "error iterating on range %s:%s", r.startKey, r.endKey)
	}

	dbKey := string(item.Key())
	dbKey = dbKey[strings.Index(dbKey, keys.NamespaceSeparator)+1:]

	r.it.Next()

	return &driver.VersionedRead{
		Key:          dbKey,
		Block:        v.Block,
		IndexInBlock: int(v.Txnum),
		Raw:          v.Value,
	}, nil
}

func (r *rangeScanIterator) Close() {
	r.it.Close()
	r.txn.Discard()
}

func (db *badgerDB) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (driver.VersionedResultsIterator, error) {
	txn := db.db.NewTransaction(false)
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	it.Seek([]byte(dbKey(namespace, startKey)))

	return &rangeScanIterator{
		txn:       txn,
		it:        it,
		startKey:  startKey,
		endKey:    endKey,
		namespace: namespace,
	}, nil
}

type ItemCache interface {
	Get(index int) (*driver.VersionedRead, bool)
	Set(index int, v *driver.VersionedRead)
}

type cachedRangeScanIterator struct {
	txn       *badger.Txn
	it        *badger.Iterator
	startKey  string
	endKey    string
	namespace string
	cache     cache

	compareKey []byte
	index      int
	items      ItemCache
}

func newCachedRangeScanIterator(
	txn *badger.Txn,
	it *badger.Iterator,
	startKey string,
	endKey string,
	namespace string,
	cache cache,
	items ItemCache,
) *cachedRangeScanIterator {
	return &cachedRangeScanIterator{
		txn:        txn,
		it:         it,
		startKey:   startKey,
		endKey:     endKey,
		namespace:  namespace,
		cache:      cache,
		compareKey: []byte(dbKey(namespace, endKey)),
		items:      items,
	}
}

func (r *cachedRangeScanIterator) Next() (*driver.VersionedRead, error) {
	if !r.it.Valid() {
		return nil, nil
	}

	v, ok := r.items.Get(r.index)
	if ok {
		return v, nil
	}

	item := r.it.Item()
	if r.endKey != "" && (bytes.Compare(item.Key(), r.compareKey) >= 0) {
		return nil, nil
	}
	v, err := r.versionedValue(item, string(item.Key()))
	if err != nil {
		return nil, errors.Wrapf(err, "error iterating on range %s:%s", r.startKey, r.endKey)
	}
	r.it.Next()

	r.items.Set(r.index, v)
	r.index++

	return v, nil
}

func (r *cachedRangeScanIterator) versionedValue(item *badger.Item, dbKey string) (*driver.VersionedRead, error) {
	// check the cache first
	if v, ok := r.cache.Get(dbKey); ok {
		if v == nil {
			vdbKey := dbKey
			vdbKey = vdbKey[strings.Index(dbKey, keys.NamespaceSeparator)+1:]
			return &driver.VersionedRead{
				Key:          vdbKey,
				Block:        0,
				IndexInBlock: 0,
				Raw:          nil,
			}, nil
		}
		return v.(*driver.VersionedRead), nil
	}

	// nothing in cache, fetch
	var res *driver.VersionedRead
	err := item.Value(func(val []byte) error {
		protoValue := &dbproto.VersionedValue{}
		if err := proto.Unmarshal(val, protoValue); err != nil {
			return errors.Wrapf(err, "could not unmarshal VersionedValue for key %s", dbKey)
		}

		if protoValue.Version != dbproto.V1 {
			return errors.Errorf("invalid version, expected %d, got %d", dbproto.V1, protoValue.Version)
		}

		vdbKey := dbKey
		vdbKey = vdbKey[strings.Index(dbKey, keys.NamespaceSeparator)+1:]
		res = &driver.VersionedRead{
			Key:          vdbKey,
			Block:        protoValue.Block,
			IndexInBlock: int(protoValue.Txnum),
			Raw:          protoValue.Value,
		}

		// store in cache
		r.cache.Add(dbKey, res)

		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "could not get value for key %s", dbKey)
	}

	return res, nil
}

var CacheIteratorOptions = badger.IteratorOptions{
	PrefetchValues: true,
	PrefetchSize:   2000,
	Reverse:        false,
	AllVersions:    false,
}

func (r *cachedRangeScanIterator) Close() {
	r.it.Close()
	r.txn.Discard()
}

func (db *badgerDB) GetCachedStateRangeScanIterator(namespace string, startKey string, endKey string) (driver.VersionedResultsIterator, error) {
	txn := db.db.NewTransaction(false)
	it := txn.NewIterator(CacheIteratorOptions)
	it.Seek([]byte(dbKey(namespace, startKey)))

	db.itemsMapLock.RLock()
	itemsMap, ok := db.itemsMap[namespace+startKey+endKey]
	if !ok {
		db.itemsMapLock.RUnlock()
		db.itemsMapLock.Lock()
		itemsMap, ok = db.itemsMap[namespace+startKey+endKey]
		if !ok {
			itemsMap = &ItemList{items: make([]*driver.VersionedRead, 2000)}
			db.itemsMap[namespace+startKey+endKey] = itemsMap
		}
		db.itemsMapLock.Unlock()
	} else {
		db.itemsMapLock.RUnlock()
	}

	return newCachedRangeScanIterator(txn, it, startKey, endKey, namespace, db.cache, itemsMap), nil
}
