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

	sq "github.com/Masterminds/squirrel"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	sql2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	q "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query"
	common4 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/common"
	cond2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/cond"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/pagination"
)

var logger = logging.MustGetLogger()

type VaultTables struct {
	StateTable  string
	StatusTable string
}

type Sanitizer interface {
	EncodeAll(params []common4.Param) ([]any, error)
	Encode(string) (string, error)
	Decode(string) (string, error)
}

// NewVaultStore creates a VaultStore. ph is the squirrel PlaceholderFormat (sq.Dollar or sq.Question)
// used for all SELECT queries; write queries continue to use the legacy builder with $N placeholders.
func NewVaultStore(writeDB common3.WriteDB, readDB *sql.DB, tables VaultTables, errorWrapper driver2.SQLErrorWrapper, ci common4.CondInterpreter, ph sq.PlaceholderFormat, sanitizer sql2.Sanitizer, il common3.IsolationLevelMapper) *VaultStore {
	sb := sq.StatementBuilder.PlaceholderFormat(ph)
	vaultSanitizer := common3.NewSanitizer(sanitizer)
	return &VaultStore{
		vaultReader: &vaultReader{
			readDB:    readDB,
			sb:        sb,
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
	writeDB      common3.WriteDB
	ci           common4.CondInterpreter // kept for legacy write-path methods
	GlobalLock   sync.RWMutex
	sanitizer    Sanitizer
	il           common3.IsolationLevelMapper
}

func (db *VaultStore) NewTxLockVaultReader(ctx context.Context, txID driver.TxID, isolationLevel driver.IsolationLevel) (driver.LockedVaultReader, error) {
	logger.DebugfContext(ctx, "acquire tx id lock for [%s]", txID)

	// Ignore conflicts in case replicas create the same entry when receiving the envelope
	query, params := q.InsertInto(db.tables.StatusTable).
		Fields("tx_id", "code").
		Row(txID, driver.Busy).
		OnConflictDoNothing().
		Format()
	logger.Debug(query, params)

	if _, err := db.writeDB.ExecContext(ctx, query, params...); err != nil {
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
		sb:        db.vaultReader.sb,
		sanitizer: db.sanitizer,
		tables:    db.tables,
	}, tx.Commit, nil
}

func (db *VaultStore) NewGlobalLockVaultReader(ctx context.Context) (driver.LockedVaultReader, error) {
	logger.DebugfContext(ctx, "start acquire global lock")
	defer logger.DebugfContext(ctx, "end acquire global lock")
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
		sb:        db.vaultReader.sb,
		sanitizer: db.sanitizer,
		tables:    db.tables,
	}, release, nil
}

func (db *VaultStore) SetStatuses(ctx context.Context, code driver.TxStatusCode, message string, txIDs ...driver.TxID) error {
	db.GlobalLock.RLock()
	defer db.GlobalLock.RUnlock()

	rows := make([]common4.Tuple, len(txIDs))
	for i, txID := range txIDs {
		rows[i] = common4.Tuple{txID, code, message}
	}

	query, params := q.InsertInto(db.tables.StatusTable).
		Fields("tx_id", "code", "message").
		Rows(rows).
		OnConflict([]common4.FieldName{"tx_id"},
			q.SetValue("code", code),
			q.SetValue("message", message),
		).
		Format()

	logger.Debug(query, params)

	if _, err := db.writeDB.ExecContext(ctx, query, params...); err != nil {
		return errors.Wrapf(err, "failed updating statuses for %d txids", len(txIDs))
	}
	return nil
}

func (db *VaultStore) SetStatusesBusy(txIDs []driver.TxID, sb common4.Builder) {
	rows := make([]common4.Tuple, len(txIDs))
	for i, txID := range txIDs {
		rows[i] = common4.Tuple{txID, driver.Busy}
	}

	q.InsertInto(db.tables.StatusTable).
		Fields("tx_id", "code").
		Rows(rows).
		OnConflict([]common4.FieldName{"tx_id"}, q.SetValue("code", driver.Busy)).
		FormatTo(sb)
}

func (db *VaultStore) UpsertStates(writes driver.Writes, metaWrites driver.MetaWrites, sb common4.Builder) error {
	states, err := db.convertStateRows(writes, metaWrites)
	if err != nil {
		return err
	}
	q.InsertInto(db.tables.StateTable).
		Fields("ns", "pkey", "val", "kversion", "metadata").
		Rows(states).
		OnConflict([]common4.FieldName{"ns", "pkey"},
			q.OverwriteValue("val"),
			q.OverwriteValue("kversion"),
			q.OverwriteValue("metadata"),
		).
		FormatTo(sb)
	return nil
}

func (db *VaultStore) SetStatusesValid(txIDs []driver.TxID, sb common4.Builder) {
	q.Update(db.tables.StatusTable).
		Set("code", driver.Valid).
		Where(cond2.In("tx_id", txIDs...)).
		FormatTo(db.ci, sb)
}

func (db *VaultStore) convertStateRows(writes driver.Writes, metaWrites driver.MetaWrites) ([]common4.Tuple, error) {
	states := make([]common4.Tuple, 0, len(writes))
	for ns, write := range writes {
		metaWrite, ok := metaWrites[ns]
		if !ok {
			metaWrite = map[driver.PKey]driver.VaultMetadataValue{}
		}
		for pkey, val := range write {
			metadata := make([]byte, 0)
			var metaVersion driver.RawVersion
			var err error
			if metaVal, ok := metaWrite[pkey]; ok {
				metadata, err = marshallMetadata(metaVal.Metadata)
				metaVersion = metaVal.Version
			}
			if len(metaVersion) > 0 && !bytes.Equal(val.Version, metaVersion) {
				logger.Warnf("different values passed for metadata version and data version: [%s] [%s]", metaVersion, val.Version)
			}
			if err != nil {
				return nil, errors.Wrapf(err, "failed to marshal metadata for [%s:%s]", ns, pkey)
			}
			if len(val.Raw) == 0 {
				logger.Debugf("setting version of [%s] to nil", pkey)
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
			states = append(states, common4.Tuple{ns, pkey, val.Raw, val.Version, metadata})
		}
	}

	for ns, metaWrite := range metaWrites {
		write, ok := writes[ns]
		if !ok {
			write = map[driver.PKey]driver.VaultValue{}
		}
		for pkey, metaVal := range metaWrite {
			if _, ok = write[pkey]; ok {
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
			states = append(states, common4.Tuple{ns, pkey, []byte{}, metaVal.Version, metadata})
		}
	}
	return states, nil
}

type dbReader interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
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

func (db *txVaultReader) GetAllTxStatuses(ctx context.Context, p driver.Pagination) (*driver.PageIterator[*driver.TxStatus], error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetAllTxStatuses(ctx, p)
}

func (db *txVaultReader) Done() error {
	return db.release()
}

// vaultReader handles all read queries using squirrel for SELECT statements.
type vaultReader struct {
	readDB    dbReader
	sb        sq.StatementBuilderType
	sanitizer Sanitizer
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
	return db.queryState(ctx, sq.And{sq.Eq{"ns": namespace}, sq.Eq{"pkey": keys}})
}

func (db *vaultReader) GetStateRange(ctx context.Context, namespace driver.Namespace, startKey, endKey driver.PKey) (driver.TxStateIterator, error) {
	return db.queryState(ctx, sq.And{sq.Eq{"ns": namespace}, betweenStrings("pkey", startKey, endKey)})
}

func (db *vaultReader) GetAllStates(ctx context.Context, namespace driver.Namespace) (driver.TxStateIterator, error) {
	return db.queryState(ctx, sq.Eq{"ns": namespace})
}

// betweenStrings returns a half-open [start, end) range condition, omitting a bound when it is empty.
func betweenStrings(col, start, end string) sq.Sqlizer {
	var parts sq.And
	if start != "" {
		parts = append(parts, sq.GtOrEq{col: start})
	}
	if end != "" {
		parts = append(parts, sq.Lt{col: end})
	}
	return parts
}

func (db *vaultReader) queryState(ctx context.Context, where sq.Sqlizer) (driver.TxStateIterator, error) {
	query, params, err := db.sb.Select("pkey", "kversion", "val").
		From(db.tables.StateTable).
		Where(where).
		ToSql()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build query")
	}
	encodedParams, err := db.sanitizer.EncodeAll(params)
	if err != nil {
		return nil, err
	}

	logger.Debug(query, encodedParams)
	rows, err := db.readDB.QueryContext(ctx, query, encodedParams...)
	if err != nil {
		return nil, err
	}
	return common3.NewIterator(rows, func(r *driver.VaultRead) error {
		if err := rows.Scan(&r.Key, &r.Version, &r.Raw); err != nil {
			return err
		}
		r.Key, err = db.sanitizer.Decode(r.Key)
		return err
	}), nil
}

func (db *vaultReader) GetStateMetadata(ctx context.Context, namespace driver.Namespace, key driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	namespace, err := db.sanitizer.Encode(namespace)
	if err != nil {
		return nil, nil, err
	}
	key, err = db.sanitizer.Encode(key)
	if err != nil {
		return nil, nil, err
	}

	query, params, err := db.sb.Select("metadata", "kversion").
		From(db.tables.StateTable).
		Where(sq.And{sq.Eq{"ns": namespace}, sq.Eq{"pkey": key}}).
		ToSql()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to build query")
	}
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
	// Select the row with the highest pos that is not Busy
	it, err := db.queryStatus(ctx,
		sq.Expr("pos=(SELECT max(pos) FROM "+db.tables.StatusTable+" WHERE code!=?)", driver.Busy),
		pagination.None())
	if err != nil {
		return nil, err
	}
	return collections.GetUnique(it)
}

func (db *vaultReader) GetTxStatus(ctx context.Context, txID driver.TxID) (*driver.TxStatus, error) {
	it, err := db.queryStatus(ctx, sq.Eq{"tx_id": txID}, pagination.None())
	if err != nil {
		return nil, err
	}
	return collections.GetUnique(it)
}

func (db *vaultReader) GetTxStatuses(ctx context.Context, txIDs ...driver.TxID) (driver.TxStatusIterator, error) {
	if len(txIDs) == 0 {
		return collections.NewEmptyIterator[*driver.TxStatus](), nil
	}
	return db.queryStatus(ctx, sq.Eq{"tx_id": txIDs}, pagination.None())
}

func (db *vaultReader) GetAllTxStatuses(ctx context.Context, p driver.Pagination) (*driver.PageIterator[*driver.TxStatus], error) {
	if p == nil {
		return nil, fmt.Errorf("invalid input pagination: %+v", p)
	}
	txStatusIterator, err := db.queryStatus(ctx, nil, p)
	if err != nil {
		return nil, err
	}
	return pagination.NewPage[driver.TxStatus](txStatusIterator, p)
}

func (db *vaultReader) queryStatus(ctx context.Context, where sq.Sqlizer, p driver.Pagination) (driver.TxStatusIterator, error) {
	sb := db.sb.Select("tx_id", "code", "message").From(db.tables.StatusTable)
	if where != nil {
		sb = sb.Where(where)
	}
	sb = pagination.ApplyToSquirrel(p, sb)

	query, params, err := sb.ToSql()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build query")
	}
	logger.Debug(query, params)

	rows, err := db.readDB.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, err
	}
	return common3.NewIterator(rows, func(s *driver.TxStatus) error { return rows.Scan(&s.TxID, &s.Code, &s.Message) }), nil
}

func (db *VaultStore) Close() error {
	return errors2.Join(db.writeDB.Close(), db.readDB.Close())
}

// Data marshal/unmarshal

func marshallMetadata(metadata map[string][]byte) (m []byte, err error) {
	var buf bytes.Buffer
	err = gob.NewEncoder(&buf).Encode(metadata)
	if err != nil {
		return m, err
	}
	return buf.Bytes(), nil
}

func unmarshalMetadata(input []byte) (m map[string][]byte, err error) {
	if len(input) == 0 {
		return m, err
	}

	buf := bytes.NewBuffer(input)
	decoder := gob.NewDecoder(buf)
	err = decoder.Decode(&m)
	return m, err
}
