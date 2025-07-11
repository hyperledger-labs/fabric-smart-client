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

var (
	logger = logging.MustGetLogger()
)

type VaultTables struct {
	StateTable  string
	StatusTable string
}

type Sanitizer interface {
	EncodeAll(params []common4.Param) ([]any, error)
	Encode(string) (string, error)
	Decode(string) (string, error)
}

func NewVaultStore(writeDB common3.WriteDB, readDB *sql.DB, tables VaultTables, errorWrapper driver2.SQLErrorWrapper, ci common4.CondInterpreter, pi common4.PagInterpreter, sanitizer sql2.Sanitizer, il common3.IsolationLevelMapper) *VaultStore {
	vaultSanitizer := common3.NewSanitizer(sanitizer)
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
	writeDB      common3.WriteDB
	ci           common4.CondInterpreter
	pi           common4.PagInterpreter
	GlobalLock   sync.RWMutex
	sanitizer    Sanitizer
	il           common3.IsolationLevelMapper
}

func (db *VaultStore) NewTxLockVaultReader(ctx context.Context, txID driver.TxID, isolationLevel driver.IsolationLevel) (driver.LockedVaultReader, error) {
	logger.DebugfContext(ctx, "Acquire tx id lock for [%s]", txID)

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
		ci:        db.ci,
		pi:        db.pi,
		sanitizer: db.sanitizer,
		tables:    db.tables,
	}, tx.Commit, nil
}

func (db *VaultStore) NewGlobalLockVaultReader(ctx context.Context) (driver.LockedVaultReader, error) {
	logger.DebugfContext(ctx, "Start acquire global lock")
	defer logger.DebugfContext(ctx, "End acquire global lock")
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
	ci        common4.CondInterpreter
	pi        common4.PagInterpreter
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
	return db.queryState(ctx, cond2.And(cond2.Eq("ns", namespace), cond2.In("pkey", keys...)))
}

func (db *vaultReader) GetStateRange(ctx context.Context, namespace driver.Namespace, startKey, endKey driver.PKey) (driver.TxStateIterator, error) {
	return db.queryState(ctx, cond2.And(cond2.Eq("ns", namespace), cond2.BetweenStrings("pkey", startKey, endKey)))
}

func (db *vaultReader) GetAllStates(ctx context.Context, namespace driver.Namespace) (driver.TxStateIterator, error) {
	return db.queryState(ctx, cond2.Eq("ns", namespace))
}

func (db *vaultReader) queryState(ctx context.Context, where cond2.Condition) (driver.TxStateIterator, error) {
	query, params := q.Select().FieldsByName("pkey", "kversion", "val").
		From(q.Table(db.tables.StateTable)).
		Where(where).
		Format(db.ci)
	params, err := db.sanitizer.EncodeAll(params)
	if err != nil {
		return nil, err
	}

	logger.Debug(query, params)
	rows, err := db.readDB.QueryContext(ctx, query, params...)
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

	query, params := q.Select().FieldsByName("metadata", "kversion").
		From(q.Table(db.tables.StateTable)).
		Where(cond2.And(cond2.Eq("ns", namespace), cond2.Eq("pkey", key))).
		Format(db.ci)
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
	it, err := db.queryStatus(ctx, IsLast(common4.TableName(db.tables.StatusTable)), pagination.None())
	if err != nil {
		return nil, err
	}
	return collections.GetUnique(it)
}

func IsLast(tableName common4.TableName) *isLast {
	return &isLast{tableName: tableName}
}

type isLast struct {
	tableName common4.TableName
}

func (c *isLast) WriteString(_ common4.CondInterpreter, sb common4.Builder) {
	sb.WriteString("pos=(SELECT max(pos) FROM ").
		WriteString(string(c.tableName)).WriteString(" WHERE code!=").
		WriteParam(driver.Busy).
		WriteRune(')')
}

func (db *vaultReader) GetTxStatus(ctx context.Context, txID driver.TxID) (*driver.TxStatus, error) {
	it, err := db.queryStatus(ctx, cond2.Eq("tx_id", txID), pagination.None())
	if err != nil {
		return nil, err
	}
	return collections.GetUnique(it)
}
func (db *vaultReader) GetTxStatuses(ctx context.Context, txIDs ...driver.TxID) (driver.TxStatusIterator, error) {
	if len(txIDs) == 0 {
		return collections.NewEmptyIterator[*driver.TxStatus](), nil
	}

	return db.queryStatus(ctx, cond2.In("tx_id", txIDs...), pagination.None())
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

func (db *vaultReader) queryStatus(ctx context.Context, where cond2.Condition, pagination driver.Pagination) (driver.TxStatusIterator, error) {
	query, params := q.Select().FieldsByName("tx_id", "code", "message").
		From(q.Table(db.tables.StatusTable)).
		Where(where).
		Paginated(pagination).
		FormatPaginated(db.ci, db.pi)
	logger.Debugf(query, params)

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
