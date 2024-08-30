/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"bytes"
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/atomic"
)

type Logger interface {
	Debugf(template string, args ...interface{})
	Infof(template string, args ...interface{})
	Errorf(template string, args ...interface{})
}

type TXIDStoreReader[V driver.ValidationCode] interface {
	Iterator(pos interface{}) (driver.TxIDIterator[V], error)
	Get(txID driver.TxID) (V, string, error)
}

type TXIDStore[V driver.ValidationCode] interface {
	TXIDStoreReader[V]
	Set(txID driver.TxID, code V, message string) error
	Invalidate(txID driver.TxID)
}

type TxInterceptor interface {
	driver.RWSet
	RWs() *ReadWriteSet
	Reopen(qe VersionedQueryExecutor) error
}

type Populator interface {
	Populate(rws *ReadWriteSet, rwsetBytes []byte, namespaces ...driver.Namespace) error
}

type Marshaller interface {
	Marshal(rws *ReadWriteSet) ([]byte, error)
	Append(destination *ReadWriteSet, raw []byte, nss ...string) error
}

type NewInterceptorFunc[V driver.ValidationCode] func(logger Logger, qe VersionedQueryExecutor, txidStore TXIDStoreReader[V], txid driver.TxID) TxInterceptor

type (
	VersionedPersistence     = dbdriver.VersionedPersistence
	VersionedValue           = dbdriver.VersionedValue
	VersionedRead            = dbdriver.VersionedRead
	VersionedResultsIterator = dbdriver.VersionedResultsIterator
	QueryExecutor            = dbdriver.QueryExecutor
)

var (
	DeadlockDetected   = dbdriver.DeadlockDetected
	UniqueKeyViolation = dbdriver.UniqueKeyViolation
)

// Vault models a key-value Store that can be modified by committing rwsets
type Vault[V driver.ValidationCode] struct {
	logger           Logger
	txIDStore        TXIDStore[V]
	interceptorsLock sync.RWMutex
	Interceptors     map[driver.TxID]TxInterceptor
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
	store      VersionedPersistence
	storeLock  sync.RWMutex
	vcProvider driver.ValidationCodeProvider[V]

	newInterceptor NewInterceptorFunc[V]
	populator      Populator
	metrics        *Metrics
}

// New returns a new instance of Vault
func New[V driver.ValidationCode](
	logger Logger,
	store VersionedPersistence,
	txIDStore TXIDStore[V],
	vcProvider driver.ValidationCodeProvider[V],
	newInterceptor NewInterceptorFunc[V],
	populator Populator,
	tracerProvider trace.TracerProvider,
) *Vault[V] {
	return &Vault[V]{
		logger:         logger,
		Interceptors:   make(map[driver.TxID]TxInterceptor),
		store:          store,
		txIDStore:      txIDStore,
		vcProvider:     vcProvider,
		newInterceptor: newInterceptor,
		populator:      populator,
		metrics:        NewMetrics(tracerProvider),
	}
}

func (db *Vault[V]) NewQueryExecutor() (QueryExecutor, error) {
	db.logger.Debugf("getting lock for query executor")
	db.counter.Inc()
	db.storeLock.RLock()

	db.logger.Debugf("return new query executor")
	return &directQueryExecutor[V]{
		vault: db,
	}, nil
}

func (db *Vault[V]) Status(txID driver.TxID) (V, string, error) {
	code, message, err := db.txIDStore.Get(txID)
	if err != nil {
		return db.vcProvider.NotFound(), "", nil
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

func (db *Vault[V]) DiscardTx(txID driver.TxID, message string) error {
	if _, err := db.UnmapInterceptor(txID); err != nil {
		return err
	}

	db.interceptorsLock.Lock()
	defer db.interceptorsLock.Unlock()

	return db.setValidationCode(txID, db.vcProvider.Invalid(), message)
}

func (db *Vault[V]) UnmapInterceptor(txID driver.TxID) (TxInterceptor, error) {
	db.interceptorsLock.Lock()
	defer db.interceptorsLock.Unlock()

	i, in := db.Interceptors[txID]

	if !in {
		vc, _, err := db.txIDStore.Get(txID)
		if err != nil {
			return nil, errors.Wrapf(err, "read-write set for txid %s could not be found", txID)
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

func (db *Vault[V]) CommitTX(ctx context.Context, txID driver.TxID, block driver.BlockNum, indexInBloc driver.TxNum) error {
	newCtx, span := db.metrics.Vault.Start(ctx, "commit")
	defer span.End()
	db.logger.Debugf("UnmapInterceptor [%s]", txID)
	span.AddEvent("unmap_interceptor")
	i, err := db.UnmapInterceptor(txID)
	if err != nil {
		return errors.Wrapf(err, "failed to unmap interceptor for [%s]", txID)
	}
	if i == nil {
		return errors.Errorf("cannot find rwset for [%s]", txID)
	}
	rws := i.RWs()

	db.logger.Debugf("[%s] commit in vault", txID)
	for {
		err := db.commitRWs(newCtx, txID, block, indexInBloc, rws)
		if err == nil {
			return nil
		}
		if !errors.HasCause(err, DeadlockDetected) {
			span.RecordError(err)
			// This should generate a panic
			return err
		}
		db.logger.Debugf("Deadlock detected. Retrying... [%v]", err)
	}
}

func (db *Vault[V]) commitRWs(ctx context.Context, txID driver.TxID, block driver.BlockNum, indexInBloc driver.TxNum, rws *ReadWriteSet) error {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("commit_rws")
	db.logger.Debugf("get lock [%s][%d]", txID, db.counter.Load())
	db.storeLock.Lock()
	defer db.storeLock.Unlock()

	span.AddEvent("begin_update")
	if err := db.store.BeginUpdate(); err != nil {
		return errors.Wrapf(err, "begin update in store for txid '%s' failed", txID)
	}

	span.AddEvent("set_tx_busy")
	if err := db.txIDStore.Set(txID, db.vcProvider.Busy(), ""); err != nil {
		if !errors.HasCause(err, UniqueKeyViolation) {
			return err
		}
	}

	db.logger.Debugf("parse writes [%s]", txID)
	span.AddEvent("store_writes")
	if discarded, err := db.storeWrites(ctx, rws.Writes, block, indexInBloc); err != nil {
		return errors.Wrapf(err, "failed storing writes")
	} else if discarded {
		db.logger.Infof("Discarded changes while storing writes as duplicates. Skipping...")
		db.txIDStore.Invalidate(txID)
		return nil
	}

	db.logger.Debugf("parse meta writes [%s]", txID)
	span.AddEvent("store_meta_writes")
	if discarded, err := db.storeMetaWrites(ctx, rws.MetaWrites, block, indexInBloc); err != nil {
		return errors.Wrapf(err, "failed storing meta writes")
	} else if discarded {
		db.logger.Infof("Discarded changes while storing meta writes as duplicates. Skipping...")
		db.txIDStore.Invalidate(txID)
		return nil
	}

	db.logger.Debugf("set state to valid [%s]", txID)
	span.AddEvent("set_tx_valid")
	if discarded, err := db.setTxValid(txID); err != nil {
		return errors.Wrapf(err, "failed setting tx state to valid")
	} else if discarded {
		db.logger.Infof("Discarded changes while setting tx state to valid as duplicates. Skipping...")
		return nil
	}

	span.AddEvent("commit_update")
	if err := db.store.Commit(); err != nil {
		return errors.Wrapf(err, "committing tx for txid in store '%s' failed", txID)
	}

	return nil
}

func (db *Vault[V]) setTxValid(txID driver.TxID) (bool, error) {
	err := db.txIDStore.Set(txID, db.vcProvider.Valid(), "")
	if err == nil {
		return false, nil
	}

	if err1 := db.store.Discard(); err1 != nil {
		db.logger.Errorf("got error %s; discarding caused %s", err.Error(), err1.Error())
	}

	if !errors.HasCause(err, UniqueKeyViolation) {
		db.txIDStore.Invalidate(txID)
		return true, errors.Wrapf(err, "error setting tx valid")
	}
	return true, nil
}

func (db *Vault[V]) storeMetaWrites(ctx context.Context, writes NamespaceKeyedMetaWrites, block driver.BlockNum, indexInBloc driver.TxNum) (bool, error) {
	span := trace.SpanFromContext(ctx)
	for ns, keyMap := range writes {
		for key, v := range keyMap {
			db.logger.Debugf("Store meta write [%s,%s]", ns, key)

			span.AddEvent("set_tx_metadata_state")
			if err := db.store.SetStateMetadata(ns, key, v, block, indexInBloc); err != nil {
				if err1 := db.store.Discard(); err1 != nil {
					db.logger.Errorf("got error %s; discarding caused %s", err.Error(), err1.Error())
				}

				if !errors.HasCause(err, UniqueKeyViolation) {
					return true, errors.Wrapf(err, "failed to commit metadata operation on %s:%s at height %d:%d", ns, key, block, indexInBloc)
				} else {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (db *Vault[V]) storeWrites(ctx context.Context, writes Writes, block driver.BlockNum, indexInBloc driver.TxNum) (bool, error) {
	span := trace.SpanFromContext(ctx)
	for ns, keyMap := range writes {
		for key, v := range keyMap {
			db.logger.Debugf("Store write [%s,%s,%v]", ns, key, hash.Hashable(v).String())
			var err error
			if len(v) != 0 {
				span.AddEvent("set_tx_state")
				err = db.store.SetState(ns, key, VersionedValue{Raw: v, Block: block, TxNum: indexInBloc})
			} else {
				span.AddEvent("delete_tx_state")
				err = db.store.DeleteState(ns, key)
			}

			if err != nil {
				if err1 := db.store.Discard(); err1 != nil {
					db.logger.Errorf("got error %s; discarding caused %s", err.Error(), err1.Error())
				}

				if !errors.HasCause(err, UniqueKeyViolation) {
					return true, errors.Wrapf(err, "failed to commit operation on [%s:%s] at height [%d:%d]", ns, key, block, indexInBloc)
				} else {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (db *Vault[V]) SetDiscarded(txID driver.TxID, message string) error {
	return db.setValidationCode(txID, db.vcProvider.Invalid(), message)
}

func (db *Vault[V]) SetBusy(txID driver.TxID) error {
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

func (db *Vault[V]) NewRWSet(txID driver.TxID) (driver.RWSet, error) {
	return db.NewInspector(txID)
}

func (db *Vault[V]) NewInspector(txID driver.TxID) (TxInterceptor, error) {
	db.logger.Debugf("NewRWSet[%s][%d]", txID, db.counter.Load())
	i := db.newInterceptor(db.logger, &interceptorQueryExecutor[V]{db}, db.txIDStore, txID)

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

func (db *Vault[V]) GetRWSet(txID driver.TxID, rwsetBytes []byte) (driver.RWSet, error) {
	db.logger.Debugf("GetRWSet[%s][%d]", txID, db.counter.Load())
	i := db.newInterceptor(db.logger, &interceptorQueryExecutor[V]{db}, db.txIDStore, txID)

	if err := db.populator.Populate(i.RWs(), rwsetBytes); err != nil {
		return nil, errors.Wrapf(err, "failed populating tx [%s]", txID)
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

func (db *Vault[V]) InspectRWSet(rwsetBytes []byte, namespaces ...driver.Namespace) (driver.RWSet, error) {
	i := NewInspector()

	if err := db.populator.Populate(&i.Rws, rwsetBytes, namespaces...); err != nil {
		return nil, errors.Wrapf(err, "failed populating ephemeral txID")
	}

	return i, nil
}

func (db *Vault[V]) Match(txID driver.TxID, rwsRaw []byte) error {
	if len(rwsRaw) == 0 {
		return errors.Errorf("passed empty rwset")
	}

	db.logger.Debugf("UnmapInterceptor [%s]", txID)
	db.interceptorsLock.RLock()
	defer db.interceptorsLock.RUnlock()
	i, in := db.Interceptors[txID]
	if !in {
		return errors.Errorf("read-write set for txid %s could not be found", txID)
	}
	if !i.IsClosed() {
		return errors.Errorf("attempted to retrieve read-write set for %s when done has not been called", txID)
	}

	db.logger.Debugf("get lock [%s][%d]", txID, db.counter.Load())
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
		db.logger.Debugf("byte representation differs, but rwsets match [%s]", txID)
	}
	return nil
}

func (db *Vault[V]) Close() error {
	return db.store.Close()
}

func (db *Vault[V]) RWSExists(txID driver.TxID) bool {
	db.interceptorsLock.RLock()
	defer db.interceptorsLock.RUnlock()
	_, in := db.Interceptors[txID]
	return in
}

func (db *Vault[V]) Statuses(txIDs ...driver.TxID) ([]driver.TxValidationStatus[V], error) {
	it, err := db.txIDStore.Iterator(&driver.SeekSet{TxIDs: txIDs})
	if err != nil {
		return nil, err
	}
	statuses := make([]driver.TxValidationStatus[V], 0, len(txIDs))
	for status, err := it.Next(); status != nil; status, err = it.Next() {
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, driver.TxValidationStatus[V]{
			TxID:           status.TxID,
			ValidationCode: status.Code,
			Message:        status.Message,
		})
	}
	return statuses, nil
}

func (db *Vault[V]) setValidationCode(txID driver.TxID, code V, message string) error {
	if err := db.store.BeginUpdate(); err != nil {
		return errors.Wrapf(err, "begin update for txid '%s' failed", txID)
	}

	if err := db.txIDStore.Set(txID, code, message); err != nil {
		if !errors.HasCause(err, UniqueKeyViolation) {
			return err
		}
	}

	if err := db.store.Commit(); err != nil {
		return errors.Wrapf(err, "committing tx for txid '%s' failed", txID)
	}

	return nil
}

func (db *Vault[V]) GetExistingRWSet(txID driver.TxID) (driver.RWSet, error) {
	db.logger.Debugf("GetExistingRWSet[%s][%d]", txID, db.counter.Load())

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

func (db *Vault[V]) SetStatus(txID driver.TxID, code V) error {
	err := db.store.BeginUpdate()
	if err != nil {
		return errors.Wrapf(err, "begin update for txid '%s' failed", txID)
	}
	err = db.txIDStore.Set(txID, code, "")
	if err != nil {
		db.store.Discard()
		return err
	}
	err = db.store.Commit()
	if err != nil {
		db.store.Discard()
		return errors.Wrapf(err, "committing tx for txid '%s' failed", txID)
	}
	return nil
}
