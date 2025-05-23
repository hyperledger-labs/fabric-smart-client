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
	q "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/cond"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/pagination"
	"go.opentelemetry.io/otel/trace"
)

type VaultTables struct {
	StateTable  string
	StatusTable string
}

func NewVaultStore(writeDB WriteDB, readDB *sql.DB, tables VaultTables, errorWrapper driver2.SQLErrorWrapper, ci common2.CondInterpreter, pi common2.PagInterpreter, sanitizer Sanitizer, il IsolationLevelMapper) *VaultStore {
	vaultSanitizer := newSanitizer(sanitizer)
	return &VaultStore{
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

type VaultStore struct {
	*vaultReader
	tables       VaultTables
	errorWrapper driver2.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
	ci           common2.CondInterpreter
	pi           common2.PagInterpreter
	GlobalLock   sync.RWMutex
	sanitizer    *sanitizer
	il           IsolationLevelMapper
}

func (db *VaultStore) NewTxLockVaultReader(ctx context.Context, txID driver.TxID, isolationLevel driver.IsolationLevel) (driver.LockedVaultReader, error) {
	logger.Debugf("Acquire tx id lock for [%s]", txID)
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Start acquire TxID read lock")
	defer span.AddEvent("End acquire TxID read lock")

	logger.Debugf("Attempt to acquire lock for [%s]", txID)
	// Ignore conflicts in case replicas create the same entry when receiving the envelope
	query, params := q.InsertInto(db.tables.StatusTable).
		Fields("tx_id", "code").
		Row(txID, driver.Busy).
		OnConflictDoNothing().
		Format()
	logger.Debug(query, params)

	if _, err := db.writeDB.Exec(query, params...); err != nil {
		return nil, db.errorWrapper.WrapError(err)
	}

	return newTxVaultReader(func() (*vaultReader, releaseFunc, error) {
		return db.newTxLockVaultReader(ctx, isolationLevel)
	}), nil
}

func (db *VaultStore) newTxLockVaultReader(ctx context.Context, isolationLevel driver.IsolationLevel) (*vaultReader, releaseFunc, error) {
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

func (db *VaultStore) NewGlobalLockVaultReader(ctx context.Context) (driver.LockedVaultReader, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Start acquire global lock")
	defer span.AddEvent("End acquire global lock")
	return newTxVaultReader(db.newGlobalLockVaultReader), nil
}

func (db *VaultStore) newGlobalLockVaultReader() (*vaultReader, releaseFunc, error) {
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

func (db *VaultStore) SetStatuses(ctx context.Context, code driver.TxStatusCode, message string, txIDs ...driver.TxID) error {
	db.GlobalLock.RLock()
	defer db.GlobalLock.RUnlock()
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Start set statuses")
	defer span.AddEvent("End set statuses")

	rows := make([]common2.Tuple, len(txIDs))
	for i, txID := range txIDs {
		rows[i] = common2.Tuple{txID, code, message}
	}

	query, params := q.InsertInto(db.tables.StatusTable).
		Fields("tx_id", "code", "message").
		Rows(rows).
		OnConflict([]common2.FieldName{"tx_id"},
			q.SetValue("code", code),
			q.SetValue("message", message),
		).
		Format()

	logger.Debug(query, params)

	if _, err := db.writeDB.Exec(query, params...); err != nil {
		return errors.Wrapf(err, "failed updating statuses for %d txids", len(txIDs))
	}
	return nil
}

func (db *VaultStore) SetStatusesBusy(txIDs []driver.TxID, offset int) (string, []any) {
	rows := make([]common2.Tuple, len(txIDs))
	for i, txID := range txIDs {
		rows[i] = common2.Tuple{txID, driver.Busy}
	}

	return q.InsertInto(db.tables.StatusTable).
		Fields("tx_id", "code").
		Rows(rows).
		OnConflict([]common2.FieldName{"tx_id"}, q.SetValue("code", driver.Busy)).
		FormatWithOffset(&offset)
}

func (db *VaultStore) UpsertStates(writes driver.Writes, metaWrites driver.MetaWrites, offset int) (string, []any, error) {
	states, err := db.convertStateRows(writes, metaWrites)
	if err != nil {
		return "", nil, err
	}
	query, params := q.InsertInto(db.tables.StateTable).
		Fields("ns", "pkey", "val", "kversion", "metadata").
		Rows(states).
		OnConflict([]common2.FieldName{"ns", "pkey"},
			q.OverwriteValue("val"),
			q.OverwriteValue("kversion"),
			q.OverwriteValue("metadata"),
		).
		FormatWithOffset(&offset)
	return query, params, nil
}

func (db *VaultStore) SetStatusesValid(txIDs []driver.TxID, offset int) (string, []any) {
	return q.Update(db.tables.StatusTable).
		Set("code", driver.Valid).
		Where(cond.In("tx_id", txIDs...)).
		FormatWithOffset(db.ci, &offset)
}
func (db *VaultStore) convertStateRows(writes driver.Writes, metaWrites driver.MetaWrites) ([]common2.Tuple, error) {
	states := make([]common2.Tuple, 0, len(writes))
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
			states = append(states, common2.Tuple{ns, pkey, val.Raw, val.Version, metadata})
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
			states = append(states, common2.Tuple{ns, pkey, []byte{}, metaVal.Version, metadata})

		}
	}
	return states, nil
}

type dbReader interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
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

func (db *txVaultReader) GetAllTxStatuses(ctx context.Context, pagination driver.Pagination) (*driver.PageIterator[*driver.TxStatus], error) {
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
	ci        common2.CondInterpreter
	pi        common2.PagInterpreter
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
	return db.queryState(cond.And(cond.Eq("ns", namespace), cond.In("pkey", keys...)))
}

func (db *vaultReader) GetStateRange(ctx context.Context, namespace driver.Namespace, startKey, endKey driver.PKey) (driver.TxStateIterator, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Start get state range")
	defer span.AddEvent("End get state range")
	return db.queryState(cond.And(cond.Eq("ns", namespace), cond.BetweenStrings("pkey", startKey, endKey)))
}

func (db *vaultReader) GetAllStates(_ context.Context, namespace driver.Namespace) (driver.TxStateIterator, error) {
	return db.queryState(cond.Eq("ns", namespace))
}

func (db *vaultReader) queryState(where cond.Condition) (driver.TxStateIterator, error) {
	query, params := q.Select().FieldsByName("pkey", "kversion", "val").
		From(q.Table(db.tables.StateTable)).
		Where(where).
		Format(db.ci, db.pi)
	params, err := db.sanitizer.EncodeAll(params)
	if err != nil {
		return nil, err
	}
	logger.Infof(query, params)
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

	query, params := q.Select().FieldsByName("metadata", "kversion").
		From(q.Table(db.tables.StateTable)).
		Where(cond.And(cond.Eq("ns", namespace), cond.Eq("pkey", key))).
		Format(db.ci, db.pi)
	logger.Debug(query, params)

	row := db.readDB.QueryRowContext(ctx, query, params...)
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
	it, err := db.queryStatus(IsLast(common2.TableName(db.tables.StatusTable)), pagination.None())
	if err != nil {
		return nil, err
	}
	return collections.GetUnique(it)
}

func IsLast(tableName common2.TableName) *isLast {
	return &isLast{tableName: tableName}
}

type isLast struct {
	tableName common2.TableName
}

func (c *isLast) WriteString(_ common2.CondInterpreter, sb common2.Builder) {
	sb.WriteString("pos=(SELECT max(pos) FROM ").
		WriteString(string(c.tableName)).WriteString(" WHERE code!=").
		WriteParam(driver.Busy).
		WriteRune(')')
}

func (db *vaultReader) GetTxStatus(ctx context.Context, txID driver.TxID) (*driver.TxStatus, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_get_tx_status")
	defer span.AddEvent("end_get_tx_status")

	it, err := db.queryStatus(cond.Eq("tx_id", txID), pagination.None())
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

	return db.queryStatus(cond.In("tx_id", txIDs...), pagination.None())
}
func (db *vaultReader) GetAllTxStatuses(ctx context.Context, pagination driver.Pagination) (*driver.PageIterator[*driver.TxStatus], error) {
	if pagination == nil {
		return nil, fmt.Errorf("invalid input pagination: %+v", pagination)
	}

	txStatusIterator, err := db.queryStatus(nil, pagination)
	if err != nil {
		return nil, err
	}
	return &driver.PageIterator[*driver.TxStatus]{Items: txStatusIterator, Pagination: pagination}, nil
}

func (db *vaultReader) queryStatus(where cond.Condition, pagination driver.Pagination) (driver.TxStatusIterator, error) {
	query, params := q.Select().FieldsByName("tx_id", "code", "message").
		From(q.Table(db.tables.StatusTable)).
		Where(where).
		Paginated(pagination).
		Format(db.ci, db.pi)
	logger.Infof(query, params)

	rows, err := db.readDB.Query(query, params...)
	if err != nil {
		return nil, err
	}
	return &TxCodeIterator{rows: rows}, nil
}

func (db *VaultStore) Close() error {
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
