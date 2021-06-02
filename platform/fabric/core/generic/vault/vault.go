/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package vault

import (
	"encoding/json"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
	"go.uber.org/atomic"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
)

var logger = flogging.MustGetLogger("fabric-sdk.vault")

type TXIDStoreReader interface {
	Get(txid string) (api.ValidationCode, error)
}

type TXIDStore interface {
	TXIDStoreReader
	Set(txid string, code api.ValidationCode) error
}

type Vault struct {
	txidStore        TXIDStore
	interceptorsLock sync.Mutex
	interceptors     map[string]*Interceptor
	counter          atomic.Int32

	// the vault handles access concurrency to the store using storeLock.
	// In particular:
	// * when a directQueryExecutor is returned, it holds a read-lock;
	//   when Done is called on it, the lock is released.
	// * when an interceptor is returned (using NewRWSet (in case the
	//   transaction context is generated from nothing) or GetRWSet
	//   (in case the transaction context is received from another node)),
	//   it holds a read-lock; when Done is called on it, the lock is released.
	// * an exclusive lock is held when Commit is called.
	store     driver.VersionedPersistence
	storeLock sync.RWMutex
}

func New(store driver.VersionedPersistence, txidStore TXIDStore) *Vault {
	return &Vault{
		interceptors: make(map[string]*Interceptor),
		store:        store,
		txidStore:    txidStore,
	}
}

func (db *Vault) NewQueryExecutor() (api.QueryExecutor, error) {
	logger.Debugf("getting lock for query executor")
	db.counter.Inc()
	db.storeLock.RLock()

	logger.Debugf("return new query executor")
	return &directQueryExecutor{
		vault: db,
	}, nil
}

func (db *Vault) unmapInterceptor(txid string) (*Interceptor, error) {
	db.interceptorsLock.Lock()
	defer db.interceptorsLock.Unlock()

	i, in := db.interceptors[txid]

	if !in {
		return nil, errors.Errorf("read-write set for txid %s could not be found", txid)
	}

	if !i.closed {
		return nil, errors.Errorf("attempted to retrieve read-write set for %s when done has not been called", txid)
	}

	delete(db.interceptors, txid)

	return i, nil
}

func (db *Vault) Status(txid string) (api.ValidationCode, error) {
	code, err := db.txidStore.Get(txid)
	if err != nil {
		return 0, nil
	}

	if code != api.Unknown {
		return code, nil
	}

	db.interceptorsLock.Lock()
	defer db.interceptorsLock.Unlock()

	if _, in := db.interceptors[txid]; in {
		return api.Busy, nil
	}

	return api.Unknown, nil
}

func (db *Vault) DiscardTx(txid string) error {
	_, err := db.unmapInterceptor(txid)
	if err != nil {
		return err
	}

	err = db.store.BeginUpdate()
	if err != nil {
		return errors.WithMessagef(err, "begin update for txid '%s' failed", txid)
	}

	err = db.txidStore.Set(txid, api.Invalid)
	if err != nil {
		return err
	}

	err = db.store.Commit()
	if err != nil {
		return errors.WithMessagef(err, "committing tx for txid '%s' failed", txid)
	}

	return nil
}

func (db *Vault) CommitTX(txid string, block uint64, indexInBloc int) error {
	logger.Debugf("unmapInterceptor [%s]", txid)
	i, err := db.unmapInterceptor(txid)
	if err != nil {
		return err
	}

	logger.Debugf("get lock [%s][%d]", txid, db.counter.Load())
	db.storeLock.Lock()
	defer db.storeLock.Unlock()

	m, _ := json.Marshal(i.rws)
	logger.Debugf("committing \n[%s]\n", string(m))

	err = db.store.BeginUpdate()
	if err != nil {
		return errors.WithMessagef(err, "begin update for txid '%s' failed", txid)
	}

	logger.Debugf("parse writes [%s]", txid)
	for ns, keyMap := range i.rws.writes {
		for key, v := range keyMap {
			logger.Debugf("store write [%s,%s,%v]", ns, key, hash.Hashable(v).String())
			var err error
			if len(v) != 0 {
				err = db.store.SetState(ns, key, v, block, uint64(indexInBloc))
			} else {
				err = db.store.DeleteState(ns, key)
			}

			if err != nil {
				if err1 := db.store.Discard(); err1 != nil {
					logger.Errorf("got error %s; discarding caused %s", err.Error(), err1.Error())
				}

				return errors.Errorf("failed to commit operation on %s:%s at height %d:%d", ns, key, block, indexInBloc)
			}
		}
	}

	logger.Debugf("parse meta writes [%s]", txid)
	for ns, keyMap := range i.rws.metawrites {
		for key, v := range keyMap {
			logger.Debugf("store meta write [%s,%s]", ns, key)

			err := db.store.SetStateMetadata(ns, key, v, block, uint64(indexInBloc))
			if err != nil {
				if err1 := db.store.Discard(); err1 != nil {
					logger.Errorf("got error %s; discarding caused %s", err.Error(), err1.Error())
				}

				return errors.Errorf("failed to commit metadata operation on %s:%s at height %d:%d", ns, key, block, indexInBloc)
			}
		}
	}

	logger.Debugf("set state to valid [%s]", txid)
	err = db.txidStore.Set(txid, api.Valid)
	if err != nil {
		if err1 := db.store.Discard(); err1 != nil {
			logger.Errorf("got error %s; discarding caused %s", err.Error(), err1.Error())
		}

		return err
	}

	err = db.store.Commit()
	if err != nil {
		return errors.WithMessagef(err, "committing tx for txid '%s' failed", txid)
	}

	return nil
}

func (db *Vault) NewRWSet(txid string) (*Interceptor, error) {
	logger.Debugf("NewRWSet[%s][%d]", txid, db.counter.Load())
	i := newInterceptor(&interceptorQueryExecutor{db}, db.txidStore, txid)

	db.interceptorsLock.Lock()
	if _, in := db.interceptors[txid]; in {
		db.interceptorsLock.Unlock()
		return nil, errors.Errorf("duplicate read-write set for txid %s", txid)
	}
	db.interceptors[txid] = i
	db.interceptorsLock.Unlock()

	db.counter.Inc()
	db.storeLock.RLock()

	return i, nil
}

func (db *Vault) GetRWSet(txid string, rwsetBytes []byte) (*Interceptor, error) {
	logger.Debugf("GetRWSet[%s][%d]", txid, db.counter.Load())
	i := newInterceptor(&interceptorQueryExecutor{db}, db.txidStore, txid)

	if err := i.rws.populate(rwsetBytes, txid); err != nil {
		return nil, err
	}

	db.interceptorsLock.Lock()
	if i, in := db.interceptors[txid]; in {
		if !i.closed {
			db.interceptorsLock.Unlock()
			return nil, errors.Errorf("programming error: previous read-write set for %s has not been closed", txid)
		}
	}
	db.interceptors[txid] = i
	db.interceptorsLock.Unlock()

	db.counter.Inc()
	db.storeLock.RLock()

	return i, nil
}

func (db *Vault) InspectRWSet(rwsetBytes []byte) (*Inspector, error) {
	i := newInspector()

	if err := i.rws.populate(rwsetBytes, "ephemeral"); err != nil {
		return nil, err
	}

	return i, nil
}
