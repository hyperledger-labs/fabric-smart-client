/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	errors2 "errors"
	"fmt"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"go.opentelemetry.io/otel/trace"
)

type VaultTables struct {
	StateTable  string
	StatusTable string
}

type stateRow = [5]any

func NewVaultPersistence(writeDB WriteDB, readDB *sql.DB, tables VaultTables, errorWrapper driver2.SQLErrorWrapper, ci Interpreter, pi PaginationInterpreter, sanitizer Sanitizer, il IsolationLevelMapper) *VaultPersistence {
	vaultSanitizer := newSanitizer(sanitizer)
	return &VaultPersistence{
		vaultReader: &vaultReader{
			readDB:    readDB,
			ci:        ci,
			pi:        pi,
			sanitizer: vaultSanitizer,
			tables:    tables,
		},
		tables:       tables,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
		ci:           ci,
		sanitizer:    vaultSanitizer,
		il:           il,
	}
}

type VaultPersistence struct {
	*vaultReader
	tables       VaultTables
	errorWrapper driver2.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
	ci           Interpreter
	pi           PaginationInterpreter
	GlobalLock   sync.RWMutex
	sanitizer    *sanitizer
	il           IsolationLevelMapper
}

func (db *VaultPersistence) NewTxLockVaultReader(ctx context.Context, txID driver.TxID, isolationLevel driver.IsolationLevel) (driver.LockedVaultReader, error) {
	logger.Debugf("Acquire tx id lock for [%s]", txID)
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Start acquire TxID read lock")
	defer span.AddEvent("End acquire TxID read lock")

	logger.Debugf("Attempt to acquire lock for [%s]", txID)
	// Ignore conflicts in case replicas create the same entry when receiving the envelope
	query := fmt.Sprintf("INSERT INTO %s (tx_id, code) VALUES ($1, $2) ON CONFLICT DO NOTHING", db.tables.StatusTable)
	logger.Debug(query, txID, driver.Busy)

	if _, err := db.writeDB.Exec(query, txID, driver.Busy); err != nil {
		return nil, db.errorWrapper.WrapError(err)
	}

	return newTxVaultReader(func() (*vaultReader, releaseFunc, error) {
		return db.newTxLockVaultReader(ctx, isolationLevel)
	}), nil
}

func (db *VaultPersistence) newTxLockVaultReader(ctx context.Context, isolationLevel driver.IsolationLevel) (*vaultReader, releaseFunc, error) {
	il, err := db.il.Map(isolationLevel)
	if err != nil {
		return nil, nil, err
	}
	tx, err := db.readDB.BeginTx(ctx, &sql.TxOptions{
		Isolation: il,
		ReadOnly:  true,
	})
	if err != nil {
		return nil, nil, err
	}
	return &vaultReader{
		readDB:    tx,
		ci:        db.ci,
		pi:        db.pi,
		sanitizer: db.sanitizer,
		tables:    db.tables,
	}, tx.Commit, nil
}

func (db *VaultPersistence) NewGlobalLockVaultReader(ctx context.Context) (driver.LockedVaultReader, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Start acquire global lock")
	defer span.AddEvent("End acquire global lock")
	return newTxVaultReader(db.newGlobalLockVaultReader), nil
}

func (db *VaultPersistence) newGlobalLockVaultReader() (*vaultReader, releaseFunc, error) {
	db.GlobalLock.Lock()
	release := func() error {
		db.GlobalLock.Unlock()
		return nil
	}
	return &vaultReader{
		readDB:    db.readDB,
		ci:        db.ci,
		pi:        db.pi,
		sanitizer: db.sanitizer,
		tables:    db.tables,
	}, release, nil
}

func (db *VaultPersistence) SetStatuses(ctx context.Context, code driver.TxStatusCode, message string, txIDs ...driver.TxID) error {
	db.GlobalLock.RLock()
	defer db.GlobalLock.RUnlock()
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Start set statuses")
	defer span.AddEvent("End set statuses")

	params := make([]any, 0, len(txIDs)*3+2)
	for _, txID := range txIDs {
		params = append(params, txID, code, message)
	}
	params = append(params, code, message)

	query := fmt.Sprintf(`
		INSERT INTO %s (tx_id, code, message)
		VALUES %s
		ON CONFLICT (tx_id) DO UPDATE
		SET code=$%d, message=$%d;
		`, db.tables.StatusTable,
		GenerateParamSet(1, len(txIDs), 3),
		len(txIDs)*3+1, len(txIDs)*3+2)
	logger.Debug(query, params)

	if _, err := db.writeDB.Exec(query, params...); err != nil {
		return errors.Wrapf(err, "failed updating statuses for %d txids", len(txIDs))
	}
	return nil
}

func (db *VaultPersistence) SetStatusesBusy(txIDs []driver.TxID, offset int) (string, []any) {
	params := make([]any, 0, len(txIDs)*2+1)
	for _, txID := range txIDs {
		params = append(params, txID, driver.Busy)
	}
	params = append(params, driver.Busy)

	query := fmt.Sprintf(`
		INSERT INTO %s (tx_id, code)
		VALUES %s
		ON CONFLICT (tx_id) DO UPDATE
		SET code=$%d;
		`, db.tables.StatusTable,
		GenerateParamSet(offset, len(txIDs), 2),
		offset+len(txIDs)*2)
	return query, params
}

func (db *VaultPersistence) UpsertStates(writes driver.Writes, metaWrites driver.MetaWrites, offset int) (string, []any, error) {
	states, err := db.convertStateRows(writes, metaWrites)
	if err != nil {
		return "", nil, err
	}
	params := make([]any, 0, len(states)*5)
	for _, state := range states {
		params = append(params, state[:]...)
	}
	query := fmt.Sprintf(`
		INSERT INTO %s (ns, pkey, val, kversion, metadata)
		VALUES %s
		ON CONFLICT (ns, pkey) DO UPDATE
		SET val=excluded.val, kversion=excluded.kversion, metadata=excluded.metadata;
		`, db.tables.StateTable,
		GenerateParamSet(offset, len(states), 5))
	return query, params, nil
}

func (db *VaultPersistence) SetStatusesValid(txIDs []driver.TxID, offset int) (string, []any) {
	params := append([]any{driver.Valid}, ToAnys(txIDs)...)
	query := fmt.Sprintf(`
		UPDATE %s
		SET code=$%d
		WHERE %s;
		`, db.tables.StatusTable,
		offset,
		db.ci.InStrings("tx_id", txIDs).ToString(CopyPtr(offset+1)))
	return query, params
}
func (db *VaultPersistence) convertStateRows(writes driver.Writes, metaWrites driver.MetaWrites) ([]stateRow, error) {
	states := make([]stateRow, 0, len(writes))
	for ns, write := range writes {
		metaWrite, ok := metaWrites[ns]
		if !ok {
			metaWrite = map[driver.PKey]driver.VaultMetadataValue{}
		}
		for pkey, val := range write {
			var metadata = make([]byte, 0)
			var metaVersion driver.RawVersion
			var err error
			if metaVal, ok := metaWrite[pkey]; ok {
				metadata, err = marshallMetadata(metaVal.Metadata)
				metaVersion = metaVal.Version
			}
			if len(metaVersion) > 0 && !bytes.Equal(val.Version, metaVersion) {
				logger.Warnf("Different values passed for metadata version and data version: [%s] [%s]", metaVersion, val.Version)
			}
			if err != nil {
				return nil, errors.Wrapf(err, "failed to marshal metadata for [%s:%s]", ns, pkey)
			}
			if len(val.Raw) == 0 {
				// Deleting value
				logger.Warnf("Setting version of [%s] to nil", pkey)
				val.Version = nil
			}
			ns, err := db.sanitizer.Encode(ns)
			if err != nil {
				return nil, err
			}
			pkey, err := db.sanitizer.Encode(pkey)
			if err != nil {
				return nil, err
			}
			states = append(states, stateRow{ns, pkey, val.Raw, val.Version, metadata})
		}
	}

	for ns, metaWrite := range metaWrites {
		write, ok := writes[ns]
		if !ok {
			write = map[driver.PKey]driver.VaultValue{}
		}
		for pkey, metaVal := range metaWrite {
			if _, ok = write[pkey]; ok {
				// MetaWrites already written in the previous section
				continue
			}
			metadata, err := marshallMetadata(metaVal.Metadata)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to marshal metadata for [%s:%s]", ns, pkey)
			}
			ns, err := db.sanitizer.Encode(ns)
			if err != nil {
				return nil, err
			}
			pkey, err := db.sanitizer.Encode(pkey)
			if err != nil {
				return nil, err
			}
			states = append(states, stateRow{ns, pkey, []byte{}, metaVal.Version, metadata})

		}
	}
	return states, nil
}

type dbReader interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func newTxVaultReader(newVaultReader func() (*vaultReader, releaseFunc, error)) *txVaultReader {
	return &txVaultReader{newVaultReader: newVaultReader, release: func() error { return nil }}
}

type releaseFunc func() error

type txVaultReader struct {
	once           sync.Once
	newVaultReader func() (*vaultReader, releaseFunc, error)
	vr             *vaultReader
	release        releaseFunc
}

func (db *txVaultReader) setVaultReader() error {
	var err error
	db.once.Do(func() { db.vr, db.release, err = db.newVaultReader() })
	return err
}

func (db *txVaultReader) GetState(ctx context.Context, namespace driver.Namespace, key driver.PKey) (*driver.VaultRead, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetState(ctx, namespace, key)
}

func (db *txVaultReader) GetStates(ctx context.Context, namespace driver.Namespace, keys ...driver.PKey) (driver.TxStateIterator, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetStates(ctx, namespace, keys...)
}
func (db *txVaultReader) GetStateRange(ctx context.Context, namespace driver.Namespace, startKey, endKey driver.PKey) (driver.TxStateIterator, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetStateRange(ctx, namespace, startKey, endKey)
}
func (db *txVaultReader) GetAllStates(ctx context.Context, namespace driver.Namespace) (driver.TxStateIterator, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetAllStates(ctx, namespace)
}
func (db *txVaultReader) GetStateMetadata(ctx context.Context, namespace driver.Namespace, key driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, nil, err
	}
	return db.vr.GetStateMetadata(ctx, namespace, key)
}
func (db *txVaultReader) GetLast(ctx context.Context) (*driver.TxStatus, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetLast(ctx)
}

func (db *txVaultReader) GetTxStatus(ctx context.Context, txID driver.TxID) (*driver.TxStatus, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetTxStatus(ctx, txID)
}
func (db *txVaultReader) GetTxStatuses(ctx context.Context, txIDs ...driver.TxID) (driver.TxStatusIterator, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetTxStatuses(ctx, txIDs...)
}

func (db *txVaultReader) GetAllTxStatuses(ctx context.Context, pagination driver.Pagination) (*driver.PaginatedResponse[*driver.TxStatus], error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetAllTxStatuses(ctx, pagination)
}

func (db *txVaultReader) Done() error {
	return db.release()
}

type vaultReader struct {
	readDB    dbReader
	ci        Interpreter
	pi        PaginationInterpreter
	sanitizer *sanitizer
	tables    VaultTables
}

func (db *vaultReader) GetState(ctx context.Context, namespace driver.Namespace, key driver.PKey) (*driver.VaultRead, error) {
	it, err := db.GetStates(ctx, namespace, key)
	if err != nil {
		return nil, err
	}
	return collections.GetUnique(it)
}

func (db *vaultReader) GetStates(ctx context.Context, namespace driver.Namespace, keys ...driver.PKey) (driver.TxStateIterator, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Start get states")
	defer span.AddEvent("End get states")
	return db.queryState(Where(db.ci.And(db.ci.Cmp("ns", "=", namespace), db.ci.InStrings("pkey", keys))))
}
func (db *vaultReader) GetStateRange(ctx context.Context, namespace driver.Namespace, startKey, endKey driver.PKey) (driver.TxStateIterator, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Start get state range")
	defer span.AddEvent("End get state range")
	return db.queryState(Where(db.ci.And(db.ci.Cmp("ns", "=", namespace), db.ci.BetweenStrings("pkey", startKey, endKey))))
}
func (db *vaultReader) GetAllStates(_ context.Context, namespace driver.Namespace) (driver.TxStateIterator, error) {
	return db.queryState(Where(db.ci.Cmp("ns", "=", namespace)))
}
func (db *vaultReader) queryState(where string, params []any) (driver.TxStateIterator, error) {
	params, err := db.sanitizer.EncodeAll(params)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT pkey, kversion, val FROM %s %s", db.tables.StateTable, where)
	logger.Debug(query, params)
	rows, err := db.readDB.Query(query, params...)
	if err != nil {
		return nil, err
	}
	return &TxStateIterator{rows: rows, decoder: db.sanitizer}, nil
}

func (db *vaultReader) GetStateMetadata(ctx context.Context, namespace driver.Namespace, key driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Start get state metadata")
	defer span.AddEvent("End get state metadata")
	namespace, err := db.sanitizer.Encode(namespace)
	if err != nil {
		return nil, nil, err
	}
	key, err = db.sanitizer.Encode(key)
	if err != nil {
		return nil, nil, err
	}
	where, args := Where(db.ci.And(db.ci.Cmp("ns", "=", namespace), db.ci.Cmp("pkey", "=", key)))
	query := fmt.Sprintf("SELECT metadata, kversion FROM %s %s", db.tables.StateTable, where)
	logger.Debug(query, args)

	row := db.readDB.QueryRow(query, args...)
	var m []byte
	var kversion driver.RawVersion
	err = row.Scan(&m, &kversion)
	if err != nil && err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("error querying db: %w", err)
	}
	meta, err := unmarshalMetadata(m)
	if err != nil {
		return meta, nil, fmt.Errorf("error decoding metadata: %w", err)
	}

	return meta, kversion, err
}

func (db *vaultReader) GetLast(ctx context.Context) (*driver.TxStatus, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_get_last")
	defer span.AddEvent("end_get_last")
	it, err := db.queryStatus(fmt.Sprintf("WHERE pos=(SELECT max(pos) FROM %s WHERE code!=$1)", db.tables.StatusTable), []any{driver.Busy}, "")
	if err != nil {
		return nil, err
	}
	return collections.GetUnique(it)
}
func (db *vaultReader) GetTxStatus(ctx context.Context, txID driver.TxID) (*driver.TxStatus, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_get_tx_status")
	defer span.AddEvent("end_get_tx_status")
	were, any := Where(db.ci.Cmp("tx_id", "=", txID))
	it, err := db.queryStatus(were, any, "")
	if err != nil {
		return nil, err
	}
	return collections.GetUnique(it)
}
func (db *vaultReader) GetTxStatuses(ctx context.Context, txIDs ...driver.TxID) (driver.TxStatusIterator, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_get_tx_statuses")
	defer span.AddEvent("end_get_tx_statuses")
	if len(txIDs) == 0 {
		return collections.NewEmptyIterator[*driver.TxStatus](), nil
	}
	were, any := Where(db.ci.InStrings("tx_id", txIDs))
	return db.queryStatus(were, any, "")
}
func (db *vaultReader) GetAllTxStatuses(ctx context.Context, pagination driver.Pagination) (*driver.PaginatedResponse[*driver.TxStatus], error) {
	if pagination == nil {
		return nil, fmt.Errorf("invalid input pagination: %+v", pagination)
	}
	limit, err := db.pi.Interpret(pagination)
	if err != nil {
		return nil, err
	}
	txStatusIterator, err := db.queryStatus("", []any{}, limit)
	if err != nil {
		return nil, err
	}
	return (&driver.PaginatedResponse[*driver.TxStatus]{Items: txStatusIterator, Pagination: pagination}), nil
}

func (db *vaultReader) queryStatus(where string, params []any, limit string) (driver.TxStatusIterator, error) {
	query := fmt.Sprintf("SELECT tx_id, code, message FROM %s %s %s", db.tables.StatusTable, where, limit)
	logger.Debug(query, params)

	rows, err := db.readDB.Query(query, params...)
	if err != nil {
		return nil, err
	}
	return &TxCodeIterator{rows: rows}, nil
}

func (db *VaultPersistence) Close() error {
	return errors2.Join(db.writeDB.Close(), db.readDB.Close())
}

type TxCodeIterator struct {
	rows *sql.Rows
}

func (it *TxCodeIterator) Close() {
	_ = it.rows.Close()
}

func (it *TxCodeIterator) Next() (*driver.TxStatus, error) {
	var r driver.TxStatus
	if !it.rows.Next() {
		return nil, nil
	}
	err := it.rows.Scan(&r.TxID, &r.Code, &r.Message)
	return &r, err
}

type TxStateIterator struct {
	decoder decoder
	rows    *sql.Rows
}

func (it *TxStateIterator) Close() {
	_ = it.rows.Close()
}

func (it *TxStateIterator) Next() (*driver.VaultRead, error) {
	var r driver.VaultRead
	if !it.rows.Next() {
		return nil, nil
	}
	err := it.rows.Scan(&r.Key, &r.Version, &r.Raw)
	if err != nil {
		return nil, err
	}
	r.Key, err = it.decoder.Decode(r.Key)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Data marshal/unmarshal

func marshallMetadata(metadata map[string][]byte) (m []byte, err error) {
	var buf bytes.Buffer
	err = gob.NewEncoder(&buf).Encode(metadata)
	if err != nil {
		return
	}
	return buf.Bytes(), nil
}

func unmarshalMetadata(input []byte) (m map[string][]byte, err error) {
	if len(input) == 0 {
		return
	}

	buf := bytes.NewBuffer(input)
	decoder := gob.NewDecoder(buf)
	err = decoder.Decode(&m)
	return
}
