/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"bytes"
	"context"
	errors2 "errors"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/runner"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/atomic"
)

type Logger interface {
	Debugf(template string, args ...interface{})
	Infof(template string, args ...interface{})
	Errorf(template string, args ...interface{})
}

type TXIDStoreReader[V driver.ValidationCode] interface {
	Iterator(pos interface{}) (collections.Iterator[*driver.ByNum[V]], error)
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

	commitBatcher runner.BatchRunner[txCommitIndex]
}

// New returns a new instance of Vault
func New[V driver.ValidationCode](
	logger Logger,
	store VersionedPersistence,
	txIDStore TXIDStore[V],
	vcProvider driver.ValidationCodeProvider[V],
	newInterceptor NewInterceptorFunc[V],
	populator Populator,
	metricsProvider metrics.Provider,
	tracerProvider trace.TracerProvider,
) *Vault[V] {
	v := &Vault[V]{
		logger:         logger,
		Interceptors:   make(map[driver.TxID]TxInterceptor),
		store:          store,
		txIDStore:      txIDStore,
		vcProvider:     vcProvider,
		newInterceptor: newInterceptor,
		populator:      populator,
		metrics:        NewMetrics(metricsProvider, tracerProvider),
	}
	v.commitBatcher = runner.NewSerialRunner(v.commitTXs)
	return v
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
	m, err := db.unmapInterceptors(txID)
	if err != nil {
		return nil, err
	}
	return m[txID], nil
}

func (db *Vault[V]) unmapInterceptors(txIDs ...driver.TxID) (map[driver.TxID]TxInterceptor, error) {
	db.interceptorsLock.Lock()
	defer db.interceptorsLock.Unlock()

	result, notFound := collections.SubMap(db.Interceptors, txIDs...)

	vcs, err := db.txIDStore.Iterator(&driver.SeekSet{TxIDs: notFound})
	if err != nil {
		return nil, errors.Wrapf(err, "read-write set for txids [%v] could not be found", txIDs)
	}

	foundInStore := collections.NewSet[driver.TxID]()
	for vc, err := vcs.Next(); vc != nil; vc, err = vcs.Next() {
		if err != nil {
			return nil, errors.Wrapf(err, "read-write set for txid %s could not be found", vc.TxID)
		}
		if vc.Code == db.vcProvider.Unknown() {
			return nil, errors.Errorf("read-write set for txid %s could not be found", vc.TxID)
		}
		foundInStore.Add(vc.TxID)
	}

	if unknownTxIDs := collections.NewSet(notFound...).Minus(foundInStore); !unknownTxIDs.Empty() {
		return nil, errors.Errorf("read-write set for txid %s could not be found", unknownTxIDs.ToSlice()[0])
	}

	for txID, i := range result {
		if !i.IsClosed() {
			return nil, errors.Errorf("attempted to retrieve read-write set for %s when done has not been called", txID)
		}
		delete(db.Interceptors, txID)
	}

	return result, nil
}

type txCommitIndex struct {
	ctx         context.Context
	txID        driver.TxID
	block       driver.BlockNum
	indexInBloc driver.TxNum
}

type commitInput struct {
	txCommitIndex
	rws *ReadWriteSet
}

func (db *Vault[V]) CommitTX(ctx context.Context, txID driver.TxID, block driver.BlockNum, indexInBloc driver.TxNum) error {
	start := time.Now()
	newCtx, span := db.metrics.Vault.Start(ctx, "commit")
	defer span.End()
	err := db.commitBatcher.Run(txCommitIndex{
		ctx:         newCtx,
		txID:        txID,
		block:       block,
		indexInBloc: indexInBloc,
	})
	db.metrics.BatchedCommitDuration.Observe(time.Since(start).Seconds())
	return err
}

func (db *Vault[V]) commitTXs(txs []txCommitIndex) []error {
	db.logger.Debugf("Commit %d transactions", len(txs))
	start := time.Now()
	txIDs := make([]driver.TxID, len(txs))
	for i, tx := range txs {
		txIDs[i] = tx.txID
	}
	db.logger.Debugf("UnmapInterceptors [%v]", txIDs)
	interceptors, err := db.unmapInterceptors(txIDs...)
	if err != nil {
		return collections.Repeat(errors.Wrapf(err, "failed to unmap interceptor for [%v]", txIDs), len(txs))
	}
	if len(interceptors) != len(txs) {
		notFound := collections.Difference(txIDs, collections.Keys(interceptors))
		errs := make([]error, len(notFound))
		for i, txID := range notFound {
			errs[i] = errors.Errorf("read-write set for txid %s could not be found", txID)
		}
		return errs
	}

	inputs := make([]commitInput, len(txs))
	for i, tx := range txs {
		inputs[i] = commitInput{txCommitIndex: tx, rws: interceptors[tx.txID].RWs()}
	}

	for {
		err := db.commitRWs(inputs...)
		if err == nil {
			db.metrics.CommitDuration.Observe(time.Since(start).Seconds() / float64(len(txs)))
			return collections.Repeat[error](nil, len(txs))
		}
		if !errors.HasCause(err, DeadlockDetected) {
			// This should generate a panic
			return collections.Repeat(err, len(txs))
		}
		db.logger.Debugf("Deadlock detected. Retrying... [%v]", err)
	}
}

func (db *Vault[V]) commitRWs(inputs ...commitInput) error {
	for _, input := range inputs {
		trace.SpanFromContext(input.ctx).AddEvent("wait_store_lock")
	}
	db.storeLock.Lock()
	defer db.storeLock.Unlock()

	for _, input := range inputs {
		trace.SpanFromContext(input.ctx).AddEvent("begin_update")
	}
	if err := db.store.BeginUpdate(); err != nil {
		return errors.Wrapf(err, "begin update in store for txid %v failed", inputs)
	}

	for _, input := range inputs {
		span := trace.SpanFromContext(input.ctx)

		span.AddEvent("set_tx_busy")
		if err := db.txIDStore.Set(input.txID, db.vcProvider.Busy(), ""); err != nil {
			if !errors.HasCause(err, UniqueKeyViolation) {
				return err
			}
		}

		db.logger.Debugf("parse writes [%s]", input.txID)
		span.AddEvent("store_writes")
		if discarded, err := db.storeWrites(input.ctx, input.rws.Writes, input.block, input.indexInBloc); err != nil {
			return errors.Wrapf(err, "failed storing writes")
		} else if discarded {
			db.logger.Infof("Discarded changes while storing writes as duplicates. Skipping...")
			db.txIDStore.Invalidate(input.txID)
			return nil
		}

		db.logger.Debugf("parse meta writes [%s]", input.txID)
		span.AddEvent("store_meta_writes")
		if discarded, err := db.storeMetaWrites(input.ctx, input.rws.MetaWrites, input.block, input.indexInBloc); err != nil {
			return errors.Wrapf(err, "failed storing meta writes")
		} else if discarded {
			db.logger.Infof("Discarded changes while storing meta writes as duplicates. Skipping...")
			db.txIDStore.Invalidate(input.txID)
			return nil
		}

		db.logger.Debugf("set state to valid [%s]", input.txID)
		span.AddEvent("set_tx_valid")
		if discarded, err := db.setTxValid(input.txID); err != nil {
			return errors.Wrapf(err, "failed setting tx state to valid")
		} else if discarded {
			db.logger.Infof("Discarded changes while setting tx state to valid as duplicates. Skipping...")
			return nil
		}

	}

	for _, input := range inputs {
		trace.SpanFromContext(input.ctx).AddEvent("commit_update")
	}
	if err := db.store.Commit(); err != nil {
		return errors.Wrapf(err, "committing tx for txid in store [%v] failed", inputs)
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
		span.AddEvent("set_tx_metadata_state")
		if errs := db.store.SetStateMetadatas(ns, keyMap, block, indexInBloc); len(errs) > 0 {
			return db.discard(ns, block, indexInBloc, errs)
		}
	}
	return false, nil
}

func (db *Vault[V]) storeWrites(ctx context.Context, writes Writes, block driver.BlockNum, indexInBloc driver.TxNum) (bool, error) {
	span := trace.SpanFromContext(ctx)
	for ns, keyMap := range writes {
		span.AddEvent("set_tx_states")
		if errs := db.store.SetStates(ns, versionedValues(keyMap, block, indexInBloc)); len(errs) > 0 {
			return db.discard(ns, block, indexInBloc, errs)
		}
	}
	return false, nil
}

func versionedValues(keyMap NamespaceWrites, block driver.BlockNum, indexInBloc driver.TxNum) map[driver.PKey]VersionedValue {
	vals := make(map[driver.PKey]VersionedValue, len(keyMap))
	for pkey, val := range keyMap {
		vals[pkey] = VersionedValue{Raw: val, Block: block, TxNum: indexInBloc}
	}
	return vals
}

func (db *Vault[V]) discard(ns driver.Namespace, block driver.BlockNum, indexInBloc driver.TxNum, errs map[driver.PKey]error) (bool, error) {
	if err1 := db.store.Discard(); err1 != nil {
		db.logger.Errorf("got error %v; discarding caused %s", errors2.Join(collections.Values(errs)...), err1.Error())
	}
	for key, err := range errs {
		if !errors.HasCause(err, UniqueKeyViolation) {
			return true, errors.Wrapf(err, "failed to commit operation on [%s:%s] at height [%d:%d]", ns, key, block, indexInBloc)
		}
	}
	return true, nil
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
