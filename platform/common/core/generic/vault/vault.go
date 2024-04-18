/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"bytes"
	"sync"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/pkg/errors"
	"go.uber.org/atomic"
)

var logger = flogging.MustGetLogger("fabric-sdk.vault")

type TXIDStoreReader[V ValidationCode] interface {
	Iterator(pos interface{}) (TxIDIterator[V], error)
	Get(txID core.TxID) (V, string, error)
}

type TXIDStore[V ValidationCode] interface {
	TXIDStoreReader[V]
	Set(txID core.TxID, code V, message string) error
}

type TxInterceptor interface {
	driver2.RWSet
	IsClosed() bool
	RWs() *ReadWriteSet
	Reopen(qe QueryExecutor) error
}

// Vault models a key-value Store that can be modified by committing rwsets
type Vault[V ValidationCode] struct {
	txIDStore        TXIDStore[V]
	interceptorsLock sync.RWMutex
	Interceptors     map[core.TxID]TxInterceptor
	counter          atomic.Int32

	// the vault handles access concurrency to the Store using StoreLock.
	// In particular:
	// * when a directQueryExecutor is returned, it holds a read-lock;
	//   when Done is called on it, the lock is released.
	// * when an interceptor is returned (using NewRWSet (in case the
	//   transaction context is generated from nothing) or GetRWSet
	//   (in case the transaction context is received from another node)),
	//   it holds a read-lock; when Done is called on it, the lock is released.
	// * an exclusive lock is held when Commit is called.
	store      driver.VersionedPersistence
	storeLock  sync.RWMutex
	vcProvider ValidationCodeProvider[V]

	newInterceptor func(qe QueryExecutor, txidStore TXIDStoreReader[V], txid core.TxID) TxInterceptor
}

// New returns a new instance of Vault
func New[V ValidationCode](store driver.VersionedPersistence, txIDStore TXIDStore[V], vcProvider ValidationCodeProvider[V], newInterceptor func(qe QueryExecutor, txidStore TXIDStoreReader[V], txid core.TxID) TxInterceptor) *Vault[V] {
	return &Vault[V]{
		Interceptors:   make(map[core.TxID]TxInterceptor),
		store:          store,
		txIDStore:      txIDStore,
		vcProvider:     vcProvider,
		newInterceptor: newInterceptor,
	}
}

func (db *Vault[V]) NewQueryExecutor() (driver2.QueryExecutor, error) {
	logger.Debugf("getting lock for query executor")
	db.counter.Inc()
	db.storeLock.RLock()

	logger.Debugf("return new query executor")
	return &directQueryExecutor[V]{
		vault: db,
	}, nil
}

func (db *Vault[V]) Status(txID core.TxID) (V, string, error) {
	code, message, err := db.txIDStore.Get(txID)
	if err != nil {
		return db.vcProvider.Unknown(), "", nil // TODO: Different
	}

	if code != db.vcProvider.Unknown() {
		return code, message, nil
	}

	db.interceptorsLock.RLock()
	defer db.interceptorsLock.RUnlock()

	if _, in := db.Interceptors[txID]; in {
		return db.vcProvider.Busy(), message, nil // TODO: Different
	}

	return db.vcProvider.Unknown(), message, nil
}

func (db *Vault[V]) DiscardTx(txID core.TxID, message string) error {
	if _, err := db.UnmapInterceptor(txID); err != nil {
		return err
	}

	db.interceptorsLock.Lock()
	defer db.interceptorsLock.Unlock()

	return db.setValidationCode(txID, db.vcProvider.Invalid(), message)
}

func (db *Vault[V]) UnmapInterceptor(txID core.TxID) (TxInterceptor, error) {
	db.interceptorsLock.Lock()
	defer db.interceptorsLock.Unlock()

	i, in := db.Interceptors[txID]

	if !in {
		vc, _, err := db.txIDStore.Get(txID)
		if err != nil {
			return nil, errors.Errorf("read-write set for txid %s could not be found", txID)
		}
		if vc == db.vcProvider.Unknown() {
			return nil, errors.Errorf("read-write set for txid %s could not be found", txID)
		}
		return nil, nil
	}

	if !i.IsClosed() {
		return nil, errors.Errorf("attempted to retrieve read-write set for %s when done has not been called", txID)
	}

	delete(db.Interceptors, txID)

	return i, nil
}

func (db *Vault[V]) CommitTX(txID core.TxID, block core.BlockNum, indexInBloc int) error {
	logger.Debugf("UnmapInterceptor [%s]", txID)
	i, err := db.UnmapInterceptor(txID)
	if err != nil {
		return errors.Wrapf(err, "failed to unmap interceptor for [%s]", txID)
	}
	if i == nil {
		return errors.Errorf("cannot find rwset for [%s]", txID)
	}

	logger.Debugf("get lock [%s][%d]", txID, db.counter.Load())
	db.storeLock.Lock()
	defer db.storeLock.Unlock()

	if err := db.store.BeginUpdate(); err != nil {
		return errors.WithMessagef(err, "begin update for txid '%s' failed", txID)
	}

	logger.Debugf("parse writes [%s]", txID)
	if discarded, err := db.storeWrites(i.RWs().Writes, block, uint64(indexInBloc)); err != nil {
		return errors.Wrapf(err, "failed storing writes")
	} else if discarded {
		logger.Infof("Discarded changes while storing writes as duplicates. Skipping...")
		return nil
	}

	logger.Debugf("parse meta writes [%s]", txID)
	if discarded, err := db.storeMetaWrites(i.RWs().MetaWrites, block, uint64(indexInBloc)); err != nil {
		return errors.Wrapf(err, "failed storing meta writes")
	} else if discarded {
		logger.Infof("Discarded changes while storing meta writes as duplicates. Skipping...")
		return nil
	}

	logger.Debugf("set state to valid [%s]", txID)
	if discarded, err := db.setTxValid(txID); err != nil {
		return errors.Wrapf(err, "failed setting tx state to valid")
	} else if discarded {
		logger.Infof("Discarded changes while setting tx state to valid as duplicates. Skipping...")
		return nil
	}

	if err = db.store.Commit(); err != nil {
		return errors.WithMessagef(err, "committing tx for txid '%s' failed", txID)
	}

	return nil
}

func (db *Vault[V]) setTxValid(txID core.TxID) (bool, error) {
	err := db.txIDStore.Set(txID, db.vcProvider.Valid(), "")
	if err == nil {
		return false, nil
	}

	if err1 := db.store.Discard(); err1 != nil {
		logger.Errorf("got error %s; discarding caused %s", err.Error(), err1.Error())
	}

	if !errors2.HasCause(err, driver.UniqueKeyViolation) {
		return true, err
	}
	return true, nil
}

func (db *Vault[V]) storeMetaWrites(writes NamespaceKeyedMetaWrites, block core.BlockNum, indexInBloc core.TxNum) (bool, error) {
	for ns, keyMap := range writes {
		for key, v := range keyMap {
			logger.Debugf("Store meta write [%s,%s]", ns, key)

			if err := db.store.SetStateMetadata(ns, key, v, block, uint64(indexInBloc)); err != nil {
				if err1 := db.store.Discard(); err1 != nil {
					logger.Errorf("got error %s; discarding caused %s", err.Error(), err1.Error())
				}

				if !errors2.HasCause(err, driver.UniqueKeyViolation) {
					return true, errors.Errorf("failed to commit metadata operation on %s:%s at height %d:%d", ns, key, block, indexInBloc)
				} else {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (db *Vault[V]) storeWrites(writes Writes, block core.BlockNum, indexInBloc core.TxNum) (bool, error) {
	for ns, keyMap := range writes {
		for key, v := range keyMap {
			logger.Debugf("Store write [%s,%s,%v]", ns, key, hash.Hashable(v).String())
			var err error
			if len(v) != 0 {
				err = db.store.SetState(ns, key, v, block, indexInBloc)
			} else {
				err = db.store.DeleteState(ns, key)
			}

			if err != nil {
				if err1 := db.store.Discard(); err1 != nil {
					logger.Errorf("got error %s; discarding caused %s", err.Error(), err1.Error())
				}

				if !errors2.HasCause(err, driver.UniqueKeyViolation) {
					return true, errors.Wrapf(err, "failed to commit operation on [%s:%s] at height [%d:%d]", ns, key, block, indexInBloc)
				} else {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (db *Vault[V]) SetBusy(txID core.TxID) error {
	code, _, err := db.txIDStore.Get(txID)
	if err != nil {
		return err
	}
	if code != db.vcProvider.Unknown() {
		// nothing to set
		return nil
	}

	return db.setValidationCode(txID, db.vcProvider.Busy(), "")
}

func (db *Vault[V]) NewRWSet(txID core.TxID) (TxInterceptor, error) {
	logger.Debugf("NewRWSet[%s][%d]", txID, db.counter.Load())
	i := db.newInterceptor(&interceptorQueryExecutor[V]{db}, db.txIDStore, txID)

	db.interceptorsLock.Lock()
	if _, in := db.Interceptors[txID]; in {
		db.interceptorsLock.Unlock()
		return nil, errors.Errorf("duplicate read-write set for txid %s", txID)
	}
	if err := db.SetBusy(txID); err != nil {
		db.interceptorsLock.Unlock()
		return nil, errors.Wrapf(err, "failed to set status to busy for txid %s", txID)
	}
	db.Interceptors[txID] = i
	db.interceptorsLock.Unlock()

	db.counter.Inc()
	db.storeLock.RLock()

	return i, nil
}

func (db *Vault[V]) GetRWSet(txID core.TxID, rwsetBytes []byte) (TxInterceptor, error) {
	logger.Debugf("GetRWSet[%s][%d]", txID, db.counter.Load())
	i := db.newInterceptor(&interceptorQueryExecutor[V]{db}, db.txIDStore, txID)

	if err := i.RWs().populate(rwsetBytes, txID); err != nil {
		return nil, err
	}

	db.interceptorsLock.Lock()
	if i, in := db.Interceptors[txID]; in {
		if !i.IsClosed() {
			db.interceptorsLock.Unlock()
			return nil, errors.Errorf("programming error: previous read-write set for %s has not been closed", txID)
		}
	}
	if err := db.SetBusy(txID); err != nil {
		db.interceptorsLock.Unlock()
		return nil, errors.Wrapf(err, "failed to set status to busy for txid %s", txID)
	}
	db.Interceptors[txID] = i
	db.interceptorsLock.Unlock()

	db.counter.Inc()
	db.storeLock.RLock()

	return i, nil
}

func (db *Vault[V]) InspectRWSet(rwsetBytes []byte, namespaces ...core.Namespace) (driver2.RWSet, error) {
	i := NewInspector()

	if err := i.Rws.populate(rwsetBytes, "ephemeral", namespaces...); err != nil {
		return nil, err
	}

	return i, nil
}

func (db *Vault[V]) Match(txID core.TxID, rwsRaw []byte) error {
	if len(rwsRaw) == 0 {
		return errors.Errorf("passed empty rwset")
	}

	logger.Debugf("UnmapInterceptor [%s]", txID)
	db.interceptorsLock.RLock()
	defer db.interceptorsLock.RUnlock()
	i, in := db.Interceptors[txID]
	if !in {
		return errors.Errorf("read-write set for txid %s could not be found", txID)
	}
	if !i.IsClosed() {
		return errors.Errorf("attempted to retrieve read-write set for %s when done has not been called", txID)
	}

	logger.Debugf("get lock [%s][%d]", txID, db.counter.Load())
	db.storeLock.Lock()
	defer db.storeLock.Unlock()

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

func (db *Vault[V]) Close() error {
	return db.store.Close()
}

func (db *Vault[V]) RWSExists(txID core.TxID) bool {
	db.interceptorsLock.RLock()
	defer db.interceptorsLock.RUnlock()
	_, in := db.Interceptors[txID]
	return in
}

func (db *Vault[V]) Statuses(txIDs ...core.TxID) ([]driver2.TxValidationStatus[V], error) {
	it, err := db.txIDStore.Iterator(&SeekSet{TxIDs: txIDs})
	if err != nil {
		return nil, err
	}
	statuses := make([]driver2.TxValidationStatus[V], 0, len(txIDs))
	for status, err := it.Next(); status != nil; status, err = it.Next() {
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, driver2.TxValidationStatus[V]{
			TxID:           status.TxID,
			ValidationCode: status.Code,
			Message:        status.Message,
		})
	}
	return statuses, nil
}

func (db *Vault[V]) setValidationCode(txID core.TxID, code V, message string) error {
	if err := db.store.BeginUpdate(); err != nil {
		return errors.WithMessagef(err, "begin update for txid '%s' failed", txID)
	}

	if err := db.txIDStore.Set(txID, code, message); err != nil {
		if !errors2.HasCause(err, driver.UniqueKeyViolation) {
			return err
		}
	}

	if err := db.store.Commit(); err != nil {
		return errors.WithMessagef(err, "committing tx for txid '%s' failed", txID)
	}

	return nil
}

func (db *Vault[V]) GetExistingRWSet(txID core.TxID) (TxInterceptor, error) {
	logger.Debugf("GetExistingRWSet[%s][%d]", txID, db.counter.Load())

	db.interceptorsLock.Lock()
	interceptor, in := db.Interceptors[txID]
	if in {
		if !interceptor.IsClosed() {
			db.interceptorsLock.Unlock()
			return nil, errors.Errorf("programming error: previous read-write set for %s has not been closed", txID)
		}
		if err := interceptor.Reopen(&interceptorQueryExecutor[V]{db}); err != nil {
			db.interceptorsLock.Unlock()
			return nil, errors.Errorf("failed to reopen rwset [%s]", txID)
		}
	} else {
		db.interceptorsLock.Unlock()
		return nil, errors.Errorf("rws for [%s] not found", txID)
	}
	if err := db.SetBusy(txID); err != nil {
		db.interceptorsLock.Unlock()
		return nil, errors.Wrapf(err, "failed to set status to busy for txid %s", txID)
	}
	db.interceptorsLock.Unlock()

	db.counter.Inc()
	db.storeLock.RLock()

	return interceptor, nil
}

func (db *Vault[V]) SetStatus(txID core.TxID, code V) error {
	err := db.store.BeginUpdate()
	if err != nil {
		return errors.WithMessagef(err, "begin update for txid '%s' failed", txID)
	}
	err = db.txIDStore.Set(txID, code, "")
	if err != nil {
		db.store.Discard()
		return err
	}
	err = db.store.Commit()
	if err != nil {
		db.store.Discard()
		return errors.WithMessagef(err, "committing tx for txid '%s' failed", txID)
	}
	return nil
}
