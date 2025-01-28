/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"bytes"
	"database/sql"
	errors2 "errors"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

type VaultTables struct {
	StateTable  string
	StatusTable string
}

type stateRow = [5]any

func NewVaultPersistence(writeDB *sql.DB, readDB *sql.DB, tables VaultTables, errorWrapper driver2.SQLErrorWrapper, ci Interpreter, sanitizer Sanitizer) *VaultPersistence {
	return &VaultPersistence{
		tables:       tables,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
		ci:           ci,
		lockManager:  newLockManager(),
		sanitizer:    newSanitizer(sanitizer),
	}
}

type VaultPersistence struct {
	tables       VaultTables
	errorWrapper driver2.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      *sql.DB
	ci           Interpreter
	lockManager  *vaultLockManager
	sanitizer    *sanitizer
}

func (db *VaultPersistence) AcquireWLocks(txIDs ...driver.TxID) error {
	return db.lockManager.AcquireWLocks(txIDs...)
}

func (db *VaultPersistence) ReleaseWLocks(txIDs ...driver.TxID) {
	db.lockManager.ReleaseWLocks(txIDs...)
}

func (db *VaultPersistence) ReleaseRLocks(txIDs ...driver.TxID) {
	db.lockManager.ReleaseRLocks(txIDs...)
}

func (db *VaultPersistence) AcquireTxIDRLock(txID driver.TxID) (driver.VaultLock, error) {
	logger.Debugf("Attempt to acquire lock for [%s]", txID)
	lock, err := db.lockManager.AcquireTxIDRLock(txID)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`
		INSERT INTO %s (tx_id, code)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
		`, db.tables.StatusTable)
	logger.Debug(query, txID, driver.Busy)

	if _, err := db.writeDB.Exec(query, txID, driver.Busy); err != nil {
		if err1 := lock.Release(); err1 != nil {
			return nil, errors2.Join(err, err1)
		}
		return nil, err
	}
	logger.Debugf("Locked [%s] successfully", txID)
	return lock, nil
}

func (db *VaultPersistence) AcquireGlobalLock() (driver.VaultLock, error) {
	return db.lockManager.AcquireGlobalRLock()
}

func (db *VaultPersistence) GetStateMetadata(namespace driver.Namespace, key driver.PKey) (driver.Metadata, driver.RawVersion, error) {
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
func (db *VaultPersistence) GetState(namespace driver.Namespace, key driver.PKey) (*driver.VersionedRead, error) {
	it, err := db.GetStates(namespace, key)
	if err != nil {
		return nil, err
	}
	return collections.GetUnique(it)
}
func (db *VaultPersistence) GetStates(namespace driver.Namespace, keys ...driver.PKey) (driver.TxStateIterator, error) {
	return db.queryState(Where(db.ci.And(db.ci.Cmp("ns", "=", namespace), db.ci.InStrings("pkey", keys))))
}
func (db *VaultPersistence) GetStateRange(namespace driver.Namespace, startKey, endKey driver.PKey) (driver.TxStateIterator, error) {
	return db.queryState(Where(db.ci.And(db.ci.Cmp("ns", "=", namespace), db.ci.BetweenStrings("pkey", startKey, endKey))))
}
func (db *VaultPersistence) GetAllStates(namespace driver.Namespace) (driver.TxStateIterator, error) {
	return db.queryState(Where(db.ci.Cmp("ns", "=", namespace)))
}

func (db *VaultPersistence) queryState(where string, params []any) (driver.TxStateIterator, error) {
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

func (db *VaultPersistence) SetStatuses(code driver.TxStatusCode, message string, txIDs ...driver.TxID) error {
	if err := db.AcquireWLocks(txIDs...); err != nil {
		return err
	}
	defer db.ReleaseWLocks(txIDs...)

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

func (db *VaultPersistence) UpdateStatusesValid(txIDs []driver.TxID, offset int) (string, []any) {
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
			metaWrite = map[driver.PKey]driver2.VersionedMetadataValue{}
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
			write = map[driver.PKey]driver2.VersionedValue{}
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

func (db *VaultPersistence) GetLast() (*driver.TxStatus, error) {
	it, err := db.queryStatus(fmt.Sprintf("WHERE pos=(SELECT max(pos) FROM %s WHERE code!=$1)", db.tables.StatusTable), []any{driver.Busy})
	if err != nil {
		return nil, err
	}
	return collections.GetUnique(it)
}
func (db *VaultPersistence) GetTxStatus(txID driver.TxID) (*driver.TxStatus, error) {
	it, err := db.queryStatus(Where(db.ci.Cmp("tx_id", "=", txID)))
	if err != nil {
		return nil, err
	}
	return collections.GetUnique(it)
}
func (db *VaultPersistence) GetTxStatuses(txIDs ...driver.TxID) (driver.TxStatusIterator, error) {
	if len(txIDs) == 0 {
		return collections.NewEmptyIterator[*driver.TxStatus](), nil
	}
	return db.queryStatus(Where(db.ci.InStrings("tx_id", txIDs)))
}
func (db *VaultPersistence) GetAllTxStatuses() (driver.TxStatusIterator, error) {
	return db.queryStatus("", []any{})
}

func (db *VaultPersistence) Close() error {
	return errors2.Join(db.writeDB.Close(), db.readDB.Close())
}

func (db *VaultPersistence) queryStatus(where string, params []any) (driver.TxStatusIterator, error) {
	query := fmt.Sprintf("SELECT tx_id, code, message FROM %s %s", db.tables.StatusTable, where)
	logger.Debug(query, params)

	rows, err := db.readDB.Query(query, params...)
	if err != nil {
		return nil, err
	}
	return &TxCodeIterator{rows: rows}, nil
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

func (it *TxStateIterator) Next() (*driver.VersionedRead, error) {
	var r driver.VersionedRead
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
