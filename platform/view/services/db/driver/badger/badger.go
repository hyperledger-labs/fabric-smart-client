/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badger

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"

	"github.com/dgraph-io/badger/v3"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	dbproto "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/keys"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("db.driver.badger")

type badgerDB struct {
	db            *badger.DB
	txn           *badger.Txn
	txnLock       sync.RWMutex
	cancelCleaner context.CancelFunc
}

const (
	defaultGCInterval     = 5 * time.Minute // TODO let the user define this via config
	defaultGCDiscardRatio = 0.5             // recommended ratio by badger docs
)

func OpenDB(opts Opts, config driver.Config) (*badgerDB, error) {
	if len(opts.Path) == 0 {
		return nil, errors.Errorf("path cannot be empty")
	}

	// let's pass our logger badger
	opt := badger.DefaultOptions(opts.Path)
	opt.Logger = logger
	copy(&opt, opts, config)

	db, err := badger.Open(opt)
	if err != nil {
		return nil, errors.Wrapf(err, "could not open DB at '%s'", opts.Path)
	}

	// count number of key
	counter := uint64(0)
	if err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			it.Item()
			counter++
		}
		return nil
	}); err != nil {
		return nil, errors.Wrapf(err, "failed to count number of keys")
	}
	logger.Debugf("badger db at [%s] contains [%d] keys", opts.Path, counter)

	// start our auto cleaner
	cancel := autoCleaner(db, defaultGCInterval, defaultGCDiscardRatio)

	return &badgerDB{db: db, cancelCleaner: cancel}, nil
}

//go:generate counterfeiter -o mock/badger.go -fake-name BadgerDB . badgerDBInterface

// badgerDBInterface exists mainly for testing the auto cleaner
type badgerDBInterface interface {
	IsClosed() bool
	RunValueLogGC(discardRatio float64) error
	Opts() badger.Options
}

// autoCleaner runs badger garbage collection periodically as long as the db is open
func autoCleaner(db badgerDBInterface, badgerGCInterval time.Duration, badgerDiscardRatio float64) context.CancelFunc {
	if db == nil || db.Opts().InMemory {
		// not needed when we run badger in memory mode
		return nil
	}

	ctx, chancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(badgerGCInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if db.IsClosed() {
					// no need to clean anymore
					return
				}
				if err := db.RunValueLogGC(badgerDiscardRatio); err != nil {
					switch err {
					case badger.ErrRejected:
						logger.Warnf("badger: value log garbage collection rejected")
					case badger.ErrNoRewrite:
						// do nothing
					default:
						logger.Warnf("badger: unexpected error while performing value log clean up: %s", err)
					}
				}
				// continue with the next tick to clean up again
			}
		}
	}()

	return chancel
}

func (db *badgerDB) Close() error {

	// TODO: what to do with db.txn if it's not nil?

	err := db.db.Close()
	if err != nil {
		return errors.Wrap(err, "could not close DB")
	}

	// stop our auto cleaner if we have one
	if db.cancelCleaner != nil {
		db.cancelCleaner()
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
	db.txn = nil

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
	if len(value) == 0 {
		logger.Warnf("set key [%s:%d:%d] to nil value, will be deleted instead", key, block, txnum)
		return db.DeleteState(namespace, key)
	}

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
