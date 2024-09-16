/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badger

import (
	"context"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	keys2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/keys"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

const (
	BadgerPersistence driver2.PersistenceType = "badger"
	FilePersistence   driver2.PersistenceType = "file"
)

var logger = flogging.MustGetLogger("db.driver.badger")

type Txn struct {
	*badger.Txn
}

func (t *Txn) Rollback() error {
	t.Txn.Discard()
	return nil
}

type DB struct {
	*common.BaseDB[*Txn]
	db            *badger.DB
	cancelCleaner context.CancelFunc
}

func OpenDB(opts Opts, config driver.Config) (*DB, error) {
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

	return &DB{db: db, cancelCleaner: cancel, BaseDB: common.NewBaseDB[*Txn](func() (*Txn, error) {
		return &Txn{db.NewTransaction(true)}, nil
	})}, nil
}

func (db *DB) Close() error {

	// TODO: what to do with db.Txn if it's not nil?

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

func (db *DB) SetState(namespace driver2.Namespace, key string, value driver.VersionedValue) error {
	if len(value.Raw) == 0 {
		logger.Warnf("set key [%s:%d:%d] to nil value, will be deleted instead", key, value.Block, value.TxNum)
		return db.DeleteState(namespace, key)
	}

	if db.Txn == nil {
		panic("programming error, writing without ongoing update")
	}

	dbKey := dbKey(namespace, key)

	v, err := txVersionedValue(db.Txn, dbKey)
	if err != nil {
		return err
	}

	v.Value = value.Raw
	v.Block = value.Block
	v.Txnum = value.TxNum

	bytes, err := proto.Marshal(v)
	if err != nil {
		return errors.Wrapf(err, "could not marshal VersionedValue for key %s", dbKey)
	}

	err = db.Txn.Set([]byte(dbKey), bytes)
	if err != nil {
		return errors.Wrapf(err, "could not set value for key %s", dbKey)
	}

	return nil
}

func (db *DB) SetStateMetadata(namespace, key string, metadata map[string][]byte, block, txnum uint64) error {
	if db.Txn == nil {
		panic("programming error, writing without ongoing update")
	}

	dbKey := dbKey(namespace, key)

	v, err := txVersionedValue(db.Txn, dbKey)
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

	err = db.Txn.Set([]byte(dbKey), bytes)
	if err != nil {
		return errors.Wrapf(err, "could not set value for key %s", dbKey)
	}

	return nil
}

func (db *DB) DeleteState(namespace, key string) error {
	if db.Txn == nil {
		panic("programming error, writing without ongoing update")
	}

	dbKey := dbKey(namespace, key)

	err := db.Txn.Delete([]byte(dbKey))
	if err != nil {
		return errors.Wrapf(err, "could not delete value for key %s", dbKey)
	}

	return nil
}

func (db *DB) GetState(namespace driver2.Namespace, key string) (driver.VersionedValue, error) {
	dbKey := dbKey(namespace, key)

	txn := &Txn{db.db.NewTransaction(false)}
	defer txn.Discard()

	v, err := txVersionedValue(txn, dbKey)
	if err != nil {
		return driver.VersionedValue{}, err
	}

	return driver.VersionedValue{Raw: v.Value, Block: v.Block, TxNum: v.Txnum}, err
}

func (db *DB) GetStateSetIterator(ns string, keys ...string) (driver.VersionedResultsIterator, error) {
	reads := make([]*driver.VersionedRead, len(keys))
	for i, key := range keys {
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
	return &keys2.DummyVersionedIterator{Items: reads}, nil
}

func (db *DB) GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error) {
	dbKey := dbKey(namespace, key)

	txn := &Txn{db.db.NewTransaction(false)}
	defer txn.Discard()

	v, err := txVersionedValue(txn, dbKey)
	if err != nil {
		return nil, 0, 0, err
	}

	return v.Meta, v.Block, v.Txnum, nil
}

func (db *DB) NewWriteTransaction() (driver.WriteTransaction, error) {
	if err := db.BeginUpdate(); err != nil {
		return nil, err
	}
	txn := &Txn{db.db.NewTransaction(true)}
	retryRunner := utils.NewRetryRunner(3, 100*time.Millisecond, true)
	return &WriteTransaction{
		db:          db.db,
		txn:         txn,
		retryRunner: retryRunner,
		txMgr:       db,
	}, nil
}

type txMgr interface {
	Commit() error
	Discard() error
}

type WriteTransaction struct {
	txMgr
	db          *badger.DB
	txn         *Txn
	retryRunner utils.RetryRunner
}

func (w *WriteTransaction) SetState(namespace driver2.Namespace, key string, value driver.VersionedValue) error {
	if w.txn == nil {
		panic("programming error, writing without ongoing update")
	}

	dbKey := dbKey(namespace, key)

	v, err := txVersionedValue(w.txn, dbKey)
	if err != nil {
		return err
	}

	v.Value = value.Raw
	v.Block = value.Block
	v.Txnum = value.TxNum

	bytes, err := proto.Marshal(v)
	if err != nil {
		return errors.Wrapf(err, "could not marshal VersionedValue for key %s", dbKey)
	}

	err = w.txn.Set([]byte(dbKey), bytes)
	if err != nil {
		return errors.Wrapf(err, "could not set value for key %s", dbKey)
	}

	return nil
}

func (w *WriteTransaction) DeleteState(namespace driver2.Namespace, key string) error {
	dbKey := dbKey(namespace, key)

	err := w.txn.Delete([]byte(dbKey))
	if err != nil {
		return errors.Wrapf(err, "could not delete value for key %s", dbKey)
	}

	return nil
}

func (w *WriteTransaction) Commit() error {
	if err := w.txn.Commit(); err != nil {
		return err
	}
	w.txn = nil
	return w.txMgr.Commit()
}

func (w *WriteTransaction) Discard() error {
	w.txn.Discard()
	w.txn = nil
	return w.txMgr.Discard()
}
