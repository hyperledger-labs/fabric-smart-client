/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/runner"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	api2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"go.opentelemetry.io/otel/trace"
)

type Logger = logging.Logger

type TxInterceptor interface {
	driver.RWSet
	RWs() *ReadWriteSet
	Reopen(qe VersionedQueryExecutor) error
}

type Populator interface {
	Populate(rwsetBytes []byte, namespaces ...driver.Namespace) (ReadWriteSet, error)
}

type Marshaller interface {
	Marshal(txID string, rws *ReadWriteSet) ([]byte, error)
	Append(destination *ReadWriteSet, raw []byte, nss ...string) error
}

type TxStatusStore interface {
	GetTxStatus(ctx context.Context, txID driver.TxID) (*driver.TxStatus, error)
}

type NewInterceptorFunc[V driver.ValidationCode] func(logger Logger, ctx context.Context, rwSet ReadWriteSet, qe VersionedQueryExecutor, vaultStore TxStatusStore, txid driver.TxID) TxInterceptor

type (
	VersionedPersistence     = dbdriver.VersionedPersistence
	VersionedValue           = dbdriver.VersionedValue
	VersionedMetadataValue   = dbdriver.VersionedMetadataValue
	VersionedRead            = dbdriver.VersionedRead
	VersionedResultsIterator = dbdriver.VersionedResultsIterator
	QueryExecutor            = dbdriver.QueryExecutor
)

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

var (
	DeadlockDetected   = dbdriver.DeadlockDetected
	UniqueKeyViolation = dbdriver.UniqueKeyViolation
)

// Vault models a key-value Store that can be modified by committing rwsets
type Vault[V driver.ValidationCode] struct {
	logger           Logger
	interceptorsLock sync.RWMutex
	interceptors     map[driver.TxID]TxInterceptor

	vcProvider driver.ValidationCodeProvider[V]

	newInterceptor NewInterceptorFunc[V]
	populator      Populator
	metrics        *Metrics

	commitBatcher runner.BatchRunner[txCommitIndex]
	rwMapper      *rwSetMapper
	vaultStore    vault.CachedVaultStore
	tracer        trace.Tracer
}

// New returns a new instance of Vault
func New[V driver.ValidationCode](
	logger Logger,
	vaultStore vault.CachedVaultStore,
	vcProvider driver.ValidationCodeProvider[V],
	newInterceptor NewInterceptorFunc[V],
	populator Populator,
	metricsProvider metrics.Provider,
	tracerProvider trace.TracerProvider,
	versionBuilder VersionBuilder,
) *Vault[V] {
	v := &Vault[V]{
		logger:         logger,
		interceptors:   make(map[driver.TxID]TxInterceptor),
		vcProvider:     vcProvider,
		newInterceptor: newInterceptor,
		populator:      populator,
		metrics:        NewMetrics(metricsProvider),
		rwMapper:       &rwSetMapper{vb: versionBuilder, logger: logger},
		vaultStore:     vaultStore,
		tracer:         tracerProvider.Tracer("vault", tracing.WithMetricsOpts(tracing.MetricsOpts{Namespace: "core"})),
	}
	v.commitBatcher = runner.NewSerialRunner(v.commitTXs)
	return v
}

func (db *Vault[V]) NewQueryExecutor(ctx context.Context) (QueryExecutor, error) {
	return newGlobalLockQueryExecutor(ctx, db.vaultStore)
}

func (db *Vault[V]) Status(ctx context.Context, txID driver.TxID) (V, string, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_status")
	defer span.AddEvent("end_status")
	tx, err := db.vaultStore.GetTxStatus(ctx, txID)
	if err != nil || tx == nil {
		return db.vcProvider.FromInt32(driver.Unknown), "", err
	}
	return db.vcProvider.FromInt32(tx.Code), tx.Message, nil
}

func (db *Vault[V]) DiscardTx(ctx context.Context, txID driver.TxID, message string) error {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_discard_tx")
	defer span.AddEvent("end_discard_tx")
	db.interceptorsLock.Lock()
	defer db.interceptorsLock.Unlock()

	if _, err := db.UnmapInterceptor(txID); err != nil {
		return err
	}
	return db.vaultStore.SetStatuses(ctx, driver.Invalid, message, txID)
}

func (db *Vault[V]) UnmapInterceptor(txID driver.TxID) (TxInterceptor, error) {
	m, err := db.unmapInterceptors(txID)
	if err != nil {
		return nil, err
	}
	return m[txID], nil
}

func (db *Vault[V]) unmapInterceptors(txIDs ...driver.TxID) (map[driver.TxID]TxInterceptor, error) {
	result, notFound := collections.SubMap(db.interceptors, txIDs...)

	if len(notFound) > 0 {
		return nil, errors.Errorf("read-write set for txids [%v] could not be found", notFound)
	}

	for txID, i := range result {
		if !i.IsClosed() {
			return nil, errors.Errorf("attempted to retrieve read-write set for %s when done has not been called", txID)
		}
		delete(db.interceptors, txID)
	}

	return result, nil
}

func (db *Vault[V]) CommitTX(ctx context.Context, txID driver.TxID, block driver.BlockNum, indexInBloc driver.TxNum) error {
	start := time.Now()
	err := db.commitBatcher.Run(txCommitIndex{
		ctx:         ctx,
		txID:        txID,
		block:       block,
		indexInBloc: indexInBloc,
	})
	db.metrics.BatchedCommitDuration.Observe(time.Since(start).Seconds())
	return err
}

func (db *Vault[V]) commitTXs(txs []txCommitIndex) []error {
	ctx, span := db.tracer.Start(context.Background(), "batch_commit_txs")
	defer span.End()
	db.logger.Infof("Commit %d transactions", len(txs))
	start := time.Now()
	txIDs := make([]driver.TxID, len(txs))
	for i, tx := range txs {
		txIDs[i] = tx.txID
	}
	db.logger.Infof("UnmapInterceptors [%v]", txIDs)
	db.interceptorsLock.Lock()
	interceptors, err := db.unmapInterceptors(txIDs...)
	db.interceptorsLock.Unlock()
	if err != nil {
		return collections.Repeat(errors.Wrapf(err, "failed to unmap interceptor for [%v]", txIDs), len(txs))
	}

	inputs := make([]commitInput, len(txs))
	for i, tx := range txs {
		inputs[i] = commitInput{txCommitIndex: tx, rws: interceptors[tx.txID].RWs()}
		span.AddLink(trace.LinkFromContext(tx.ctx))
	}

	for {
		err := db.commitRWs(ctx, inputs...)
		if err == nil {
			db.metrics.CommitDuration.Observe(time.Since(start).Seconds() / float64(len(txs)))
			return collections.Repeat[error](nil, len(txs))
		}
		if !errors.HasCause(err, DeadlockDetected) {
			// This should generate a panic
			return collections.Repeat(err, len(txs))
		}
		db.logger.Infof("Deadlock detected. Retrying... [%v]", err)
	}
}

func (db *Vault[V]) commitRWs(ctx context.Context, inputs ...commitInput) error {
	for _, input := range inputs {
		trace.SpanFromContext(input.ctx).AddEvent("begin_update")
	}

	db.logger.Debugf("extract txids from [%d] inputs", len(inputs))
	txIDs := db.rwMapper.mapTxIDs(inputs)

	db.logger.Debugf("parse writes from [%d] inputs", len(inputs))
	writes, err := db.rwMapper.mapWrites(inputs)
	if err != nil {
		return err
	}

	db.logger.Debugf("parse meta writes")
	metaWrites, err := db.rwMapper.mapMetaWrites(inputs)
	if err != nil {
		return err
	}

	if err := db.vaultStore.Store(ctx, txIDs, writes, metaWrites); err != nil {
		db.vaultStore.Invalidate(txIDs...)
		return errors.Wrapf(err, "failed writing txids")
	}
	return nil
}

func (db *Vault[V]) SetDiscarded(ctx context.Context, txID driver.TxID, message string) error {
	return db.vaultStore.SetStatuses(ctx, driver.Invalid, message, txID)
}

func (db *Vault[V]) NewRWSet(ctx context.Context, txID driver.TxID) (api2.RWSet, error) {
	return db.NewInspector(ctx, txID)
}

func (db *Vault[V]) NewInspector(ctx context.Context, txID driver.TxID) (TxInterceptor, error) {
	newCtx, _ := db.tracer.Start(ctx, "inspector")
	db.logger.Debugf("NewRWSet[%s]", txID)
	qe, err := newTxLockQueryExecutor(ctx, db.vaultStore, txID)
	if err != nil {
		return nil, err
	}
	i := db.newInterceptor(db.logger, newCtx, EmptyRWSet(), qe, db.vaultStore, txID)

	db.interceptorsLock.Lock()
	defer db.interceptorsLock.Unlock()
	if _, in := db.interceptors[txID]; in {
		return nil, errors.Errorf("duplicate read-write set for txid %s", txID)
	}
	db.interceptors[txID] = i

	return i, nil
}

func (db *Vault[V]) GetRWSet(ctx context.Context, txID driver.TxID, rwsetBytes []byte) (driver.RWSet, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_get_rw_set")
	defer span.AddEvent("end_get_rw_set")
	db.logger.Debugf("GetRWSet[%s]", txID)
	rwSet, err := db.populator.Populate(rwsetBytes)
	if err != nil {
		return nil, errors.Wrapf(err, "failed populating tx [%s]", txID)
	}

	qe, err := newTxLockQueryExecutor(ctx, db.vaultStore, txID)
	if err != nil {
		return nil, err
	}
	i := db.newInterceptor(db.logger, ctx, rwSet, qe, db.vaultStore, txID)

	db.interceptorsLock.Lock()
	defer db.interceptorsLock.Unlock()
	if i, in := db.interceptors[txID]; in && !i.IsClosed() {
		return nil, errors.Errorf("programming error: previous read-write set for %s has not been closed", txID)
	}
	db.interceptors[txID] = i

	return i, nil
}

func (db *Vault[V]) InspectRWSet(_ context.Context, rwsetBytes []byte, namespaces ...driver.Namespace) (driver.RWSet, error) {
	rwSet, err := db.populator.Populate(rwsetBytes, namespaces...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed populating ephemeral txID")
	}
	return &Inspector{Rws: rwSet}, nil
}

func (db *Vault[V]) Match(ctx context.Context, txID driver.TxID, rwsRaw []byte) error {
	if len(rwsRaw) == 0 {
		return errors.Errorf("passed empty rwset")
	}

	db.logger.Debugf("UnmapInterceptor [%s]", txID)
	db.interceptorsLock.RLock()
	defer db.interceptorsLock.RUnlock()
	i, in := db.interceptors[txID]
	if !in {
		return errors.Errorf("read-write set for txid %s could not be found", txID)
	}
	if !i.IsClosed() {
		return errors.Errorf("attempted to retrieve read-write set for %s when done has not been called", txID)
	}

	db.logger.Debugf("get lock [%s]", txID)

	rwsRaw2, err := i.Bytes()
	if err != nil {
		return err
	}

	if !bytes.Equal(rwsRaw, rwsRaw2) {
		target, err := db.InspectRWSet(ctx, rwsRaw)
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
	return db.vaultStore.Close()
}

func (db *Vault[V]) RWSExists(_ context.Context, txID driver.TxID) bool {
	db.interceptorsLock.RLock()
	defer db.interceptorsLock.RUnlock()
	_, in := db.interceptors[txID]
	return in
}

func (db *Vault[V]) Statuses(ctx context.Context, txIDs ...driver.TxID) ([]driver.TxValidationStatus[V], error) {
	it, err := db.vaultStore.GetTxStatuses(ctx, txIDs...)
	if err != nil {
		return nil, err
	}

	return collections.ReadAll(collections.Map(it, func(status *driver.TxStatus) (*driver.TxValidationStatus[V], error) {
		if status == nil {
			return nil, nil
		}
		return &driver.TxValidationStatus[V]{
			TxID:           status.TxID,
			ValidationCode: db.vcProvider.FromInt32(status.Code),
			Message:        status.Message,
		}, nil
	}))
}

func (db *Vault[V]) GetExistingRWSet(ctx context.Context, txID driver.TxID) (driver.RWSet, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_get_existing_rwset")
	defer span.AddEvent("end_get_existing_rwset")
	db.logger.Debugf("GetExistingRWSet[%s]", txID)

	db.interceptorsLock.Lock()
	defer db.interceptorsLock.Unlock()
	interceptor, in := db.interceptors[txID]
	if !in {
		return nil, errors.Errorf("rws for [%s] not found", txID)
	}
	if !interceptor.IsClosed() {
		return nil, errors.Errorf("programming error: previous read-write set for %s has not been closed", txID)
	}
	qe, err := newTxLockQueryExecutor(ctx, db.vaultStore, txID)
	if err != nil {
		return nil, err
	}
	if err := interceptor.Reopen(qe); err != nil {
		return nil, errors.Errorf("failed to reopen rwset [%s]", txID)
	}

	return interceptor, nil
}

func (db *Vault[V]) SetStatus(ctx context.Context, txID driver.TxID, code V) error {
	return db.vaultStore.SetStatuses(ctx, db.vcProvider.ToInt32(code), "", txID)
}
