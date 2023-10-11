/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"bytes"
	"sync"

	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/pkg/errors"
	"go.uber.org/atomic"
)

var logger = flogging.MustGetLogger("fabric-sdk.vault")

type TXIDStoreReader interface {
	Get(txID string) (fdriver.ValidationCode, error)
}

type TXIDStore interface {
	TXIDStoreReader
	Set(txID string, code fdriver.ValidationCode) error
}

// Vault models a key-value Store that can be modified by committing rwsets
type Vault struct {
	TXIDStore        TXIDStore
	InterceptorsLock sync.RWMutex
	Interceptors     map[string]*Interceptor
	Counter          atomic.Int32

	// the vault handles access concurrency to the Store using StoreLock.
	// In particular:
	// * when a directQueryExecutor is returned, it holds a read-lock;
	//   when Done is called on it, the lock is released.
	// * when an interceptor is returned (using NewRWSet (in case the
	//   transaction context is generated from nothing) or GetRWSet
	//   (in case the transaction context is received from another node)),
	//   it holds a read-lock; when Done is called on it, the lock is released.
	// * an exclusive lock is held when Commit is called.
	Store     driver.VersionedPersistence
	StoreLock sync.RWMutex
}

// New returns a new instance of Vault
func New(store driver.VersionedPersistence, txIDStore TXIDStore) *Vault {
	return &Vault{
		Interceptors: make(map[string]*Interceptor),
		Store:        store,
		TXIDStore:    txIDStore,
	}
}

func (db *Vault) NewQueryExecutor() (fdriver.QueryExecutor, error) {
	logger.Debugf("getting lock for query executor")
	db.Counter.Inc()
	db.StoreLock.RLock()

	logger.Debugf("return new query executor")
	return &directQueryExecutor{
		vault: db,
	}, nil
}

func (db *Vault) Status(txID string) (fdriver.ValidationCode, error) {
	code, err := db.TXIDStore.Get(txID)
	if err != nil {
		return 0, nil
	}

	if code != fdriver.Unknown {
		return code, nil
	}

	db.InterceptorsLock.RLock()
	defer db.InterceptorsLock.RUnlock()

	if _, in := db.Interceptors[txID]; in {
		return fdriver.Busy, nil
	}

	return fdriver.Unknown, nil
}

func (db *Vault) DiscardTx(txID string) error {
	_, err := db.UnmapInterceptor(txID)
	if err != nil {
		return err
	}

	db.InterceptorsLock.Lock()
	defer db.InterceptorsLock.Unlock()

	err = db.Store.BeginUpdate()
	if err != nil {
		return errors.WithMessagef(err, "begin update for txid '%s' failed", txID)
	}

	err = db.TXIDStore.Set(txID, fdriver.Invalid)
	if err != nil {
		return err
	}

	err = db.Store.Commit()
	if err != nil {
		return errors.WithMessagef(err, "committing tx for txid '%s' failed", txID)
	}

	return nil
}

func (db *Vault) CommitTX(txID string, block uint64, indexInBloc int) error {
	logger.Debugf("UnmapInterceptor [%s]", txID)
	i, err := db.UnmapInterceptor(txID)
	if err != nil {
		return err
	}
	if i == nil {
		return errors.Errorf("cannot find rwset for [%s]", txID)
	}

	logger.Debugf("get lock [%s][%d]", txID, db.Counter.Load())
	db.StoreLock.Lock()
	defer db.StoreLock.Unlock()

	err = db.Store.BeginUpdate()
	if err != nil {
		return errors.WithMessagef(err, "begin update for txid '%s' failed", txID)
	}

	logger.Debugf("parse writes [%s]", txID)
	for ns, keyMap := range i.Rws.Writes {
		for key, v := range keyMap {
			logger.Debugf("Store write [%s,%s,%v]", ns, key, hash.Hashable(v).String())
			var err error
			if len(v) != 0 {
				err = db.Store.SetState(ns, key, v, block, uint64(indexInBloc))
			} else {
				err = db.Store.DeleteState(ns, key)
			}

			if err != nil {
				if err1 := db.Store.Discard(); err1 != nil {
					logger.Errorf("got error %s; discarding caused %s", err.Error(), err1.Error())
				}

				return errors.Wrapf(err, "failed to commit operation on [%s:%s] at height [%d:%d]", ns, key, block, indexInBloc)
			}
		}
	}

	logger.Debugf("parse meta writes [%s]", txID)
	for ns, keyMap := range i.Rws.MetaWrites {
		for key, v := range keyMap {
			logger.Debugf("Store meta write [%s,%s]", ns, key)

			err := db.Store.SetStateMetadata(ns, key, v, block, uint64(indexInBloc))
			if err != nil {
				if err1 := db.Store.Discard(); err1 != nil {
					logger.Errorf("got error %s; discarding caused %s", err.Error(), err1.Error())
				}

				return errors.Errorf("failed to commit metadata operation on %s:%s at height %d:%d", ns, key, block, indexInBloc)
			}
		}
	}

	logger.Debugf("set state to valid [%s]", txID)
	err = db.TXIDStore.Set(txID, fdriver.Valid)
	if err != nil {
		if err1 := db.Store.Discard(); err1 != nil {
			logger.Errorf("got error %s; discarding caused %s", err.Error(), err1.Error())
		}

		return err
	}

	err = db.Store.Commit()
	if err != nil {
		return errors.WithMessagef(err, "committing tx for txid '%s' failed", txID)
	}

	return nil
}

func (db *Vault) Close() error {
	return db.Store.Close()
}

func (db *Vault) SetBusy(txID string) error {
	code, err := db.TXIDStore.Get(txID)
	if err != nil {
		return err
	}
	if code != fdriver.Unknown {
		// nothing to set
		return nil
	}

	err = db.Store.BeginUpdate()
	if err != nil {
		return errors.WithMessagef(err, "begin update for txid '%s' failed", txID)
	}

	err = db.TXIDStore.Set(txID, fdriver.Busy)
	if err != nil {
		return err
	}

	err = db.Store.Commit()
	if err != nil {
		return errors.WithMessagef(err, "committing tx for txid '%s' failed", txID)
	}

	return nil
}

func (db *Vault) NewRWSet(txID string) (*Interceptor, error) {
	logger.Debugf("NewRWSet[%s][%d]", txID, db.Counter.Load())
	i := NewInterceptor(&interceptorQueryExecutor{db}, db.TXIDStore, txID)

	db.InterceptorsLock.Lock()
	if _, in := db.Interceptors[txID]; in {
		db.InterceptorsLock.Unlock()
		return nil, errors.Errorf("duplicate read-write set for txid %s", txID)
	}
	if err := db.SetBusy(txID); err != nil {
		db.InterceptorsLock.Unlock()
		return nil, errors.Wrapf(err, "failed to set status to busy for txid %s", txID)
	}
	db.Interceptors[txID] = i
	db.InterceptorsLock.Unlock()

	db.Counter.Inc()
	db.StoreLock.RLock()

	return i, nil
}

func (db *Vault) GetExistingRWSet(txID string) (*Interceptor, error) {
	logger.Debugf("GetExistingRWSet[%s][%d]", txID, db.Counter.Load())

	db.InterceptorsLock.Lock()
	interceptor, in := db.Interceptors[txID]
	if in {
		if !interceptor.Closed {
			db.InterceptorsLock.Unlock()
			return nil, errors.Errorf("programming error: previous read-write set for %s has not been closed", txID)
		}
		if err := interceptor.Reopen(&interceptorQueryExecutor{db}); err != nil {
			db.InterceptorsLock.Unlock()
			return nil, errors.Errorf("failed to reopen rwset [%s]", txID)
		}
	} else {
		db.InterceptorsLock.Unlock()
		return nil, errors.Errorf("")
	}
	if err := db.SetBusy(txID); err != nil {
		db.InterceptorsLock.Unlock()
		return nil, errors.Wrapf(err, "failed to set status to busy for txid %s", txID)
	}
	db.InterceptorsLock.Unlock()

	db.Counter.Inc()
	db.StoreLock.RLock()

	return interceptor, nil
}

func (db *Vault) GetRWSet(txID string, rwsetBytes []byte) (*Interceptor, error) {
	logger.Debugf("GetRWSet[%s][%d]", txID, db.Counter.Load())
	i := NewInterceptor(&interceptorQueryExecutor{db}, db.TXIDStore, txID)

	if err := i.Rws.populate(rwsetBytes, txID); err != nil {
		return nil, err
	}

	db.InterceptorsLock.Lock()
	if i, in := db.Interceptors[txID]; in {
		if !i.Closed {
			db.InterceptorsLock.Unlock()
			return nil, errors.Errorf("programming error: previous read-write set for %s has not been closed", txID)
		}
	}
	if err := db.SetBusy(txID); err != nil {
		db.InterceptorsLock.Unlock()
		return nil, errors.Wrapf(err, "failed to set status to busy for txid %s", txID)
	}
	db.Interceptors[txID] = i
	db.InterceptorsLock.Unlock()

	db.Counter.Inc()
	db.StoreLock.RLock()

	return i, nil
}

func (db *Vault) InspectRWSet(rwsetBytes []byte, namespaces ...string) (*Inspector, error) {
	i := NewInspector()

	if err := i.Rws.populate(rwsetBytes, "ephemeral", namespaces...); err != nil {
		return nil, err
	}

	return i, nil
}

func (db *Vault) Match(txID string, rwsRaw []byte) error {
	if len(rwsRaw) == 0 {
		return errors.Errorf("passed empty rwset")
	}

	logger.Debugf("UnmapInterceptor [%s]", txID)
	db.InterceptorsLock.RLock()
	defer db.InterceptorsLock.RUnlock()
	i, in := db.Interceptors[txID]
	if !in {
		return errors.Errorf("read-write set for txid %s could not be found", txID)
	}
	if !i.Closed {
		return errors.Errorf("attempted to retrieve read-write set for %s when done has not been called", txID)
	}

	logger.Debugf("get lock [%s][%d]", txID, db.Counter.Load())
	db.StoreLock.Lock()
	defer db.StoreLock.Unlock()

	rwsRaw2, err := i.Bytes()
	if err != nil {
		return err
	}

	if !bytes.Equal(rwsRaw, rwsRaw2) {
		target, err := db.InspectRWSet(rwsRaw)
		if err != nil {
			return errors.Wrapf(err, "rwsets do not match")
		}
		if err2 := i.Equals(target); err2 != nil {
			return errors.Wrapf(err2, "rwsets do not match")
		}
		// TODO: vault should support Fabric's rwset fully
		logger.Debugf("byte representation differs, but rwsets match [%s]", txID)
	}
	return nil
}

func (db *Vault) RWSExists(txID string) bool {
	db.InterceptorsLock.RLock()
	defer db.InterceptorsLock.RUnlock()
	_, in := db.Interceptors[txID]
	return in
}

func (db *Vault) UnmapInterceptor(txID string) (*Interceptor, error) {
	db.InterceptorsLock.Lock()
	defer db.InterceptorsLock.Unlock()

	i, in := db.Interceptors[txID]

	if !in {
		vc, err := db.TXIDStore.Get(txID)
		if err != nil {
			return nil, errors.Errorf("read-write set for txid %s could not be found", txID)
		}
		if vc == fdriver.Unknown {
			return nil, errors.Errorf("read-write set for txid %s could not be found", txID)
		}
		return nil, nil
	}

	if !i.Closed {
		return nil, errors.Errorf("attempted to retrieve read-write set for %s when done has not been called", txID)
	}

	delete(db.Interceptors, txID)

	return i, nil
}
