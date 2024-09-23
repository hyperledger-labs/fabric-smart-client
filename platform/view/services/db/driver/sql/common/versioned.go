/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"bytes"
	"database/sql"
	"encoding/gob"
	"fmt"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	errors2 "github.com/pkg/errors"
)

func NewVersionedReadScanner() *versionedReadScanner { return &versionedReadScanner{} }

func NewVersionedValueScanner() *versionedValueScanner { return &versionedValueScanner{} }

func NewVersionedMetadataValueScanner() *versionedMetadataValueScanner {
	return &versionedMetadataValueScanner{}
}

type versionedReadScanner struct{}

func (s *versionedReadScanner) Columns() []string { return []string{"pkey", "kversion", "val"} }

func (s *versionedReadScanner) ReadValue(txs scannable) (driver.VersionedRead, error) {
	var r driver.VersionedRead
	err := txs.Scan(&r.Key, &r.Version, &r.Raw)
	return r, err
}

type versionedValueScanner struct{}

func (s *versionedValueScanner) Columns() []string { return []string{"val", "kversion"} }

func (s *versionedValueScanner) ReadValue(txs scannable) (driver.VersionedValue, error) {
	var r driver.VersionedValue
	err := txs.Scan(&r.Raw, &r.Version)
	return r, err
}

func (s *versionedValueScanner) WriteValue(value driver.VersionedValue) []any {
	return []any{value.Raw, value.Version}
}

type versionedMetadataValueScanner struct{}

func (s *versionedMetadataValueScanner) Columns() []string {
	return []string{"metadata", "block", "txnum"}
}

func (s *versionedMetadataValueScanner) ReadValue(txs scannable) (driver2.VersionedMetadataValue, error) {
	var r driver2.VersionedMetadataValue
	var metadata []byte
	if err := txs.Scan(&metadata, &r.Block, &r.TxNum); err != nil {
		return r, err
	} else if meta, err := unmarshalMetadata(metadata); err != nil {
		return r, fmt.Errorf("error decoding metadata: %w", err)
	} else {
		r.Metadata = meta
	}
	return r, nil
}

func (s *versionedMetadataValueScanner) WriteValue(value driver2.VersionedMetadataValue) ([]any, error) {
	metadata, err := marshallMetadata(value.Metadata)
	if err != nil {
		return nil, err
	}
	return []any{metadata, value.Block, value.TxNum}, nil
}

type basePersistence[V any, R any] interface {
	driver.BasePersistence[driver.VersionedValue, driver.VersionedRead]
	hasKey(ns driver2.Namespace, pkey string) Condition
	Exists(namespace driver2.Namespace, key driver2.PKey) (bool, error)
	Exec(query string, args ...any) (sql.Result, error)
	SetStateWithTx(tx *sql.Tx, namespace driver2.Namespace, key string, value driver.VersionedValue) error
	DeleteStateWithTx(tx *sql.Tx, ns driver2.Namespace, key driver2.PKey) error
	UpsertStates(ns driver2.Namespace, valueKeys []string, vals map[driver2.PKey][]any) map[driver2.PKey]error
}

type VersionedPersistence struct {
	basePersistence[driver.VersionedValue, driver.VersionedRead]
	table           string
	errorWrapper    driver.SQLErrorWrapper
	readDB          *sql.DB
	writeDB         *sql.DB
	metadataScanner *versionedMetadataValueScanner
}

func NewVersionedPersistence(base basePersistence[driver.VersionedValue, driver.VersionedRead], table string, errorWrapper driver.SQLErrorWrapper, readDB *sql.DB, writeDB *sql.DB) *VersionedPersistence {
	return &VersionedPersistence{basePersistence: base, table: table, errorWrapper: errorWrapper, readDB: readDB, writeDB: writeDB, metadataScanner: NewVersionedMetadataValueScanner()}
}

func NewVersioned(readDB *sql.DB, writeDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *VersionedPersistence {
	base := NewBasePersistence[driver.VersionedValue, driver.VersionedRead](writeDB, readDB, table, &versionedReadScanner{}, &versionedValueScanner{}, errorWrapper, ci, writeDB.Begin)
	return NewVersionedPersistence(base, base.table, base.errorWrapper, base.readDB, base.writeDB)
}

func (db *VersionedPersistence) SetStateMetadata(ns driver2.Namespace, key driver2.PKey, metadata driver2.Metadata, block driver2.BlockNum, txnum driver2.TxNum) error {
	if len(metadata) == 0 {
		return nil
	}
	return db.SetStateMetadatas(ns, map[driver2.PKey]driver2.VersionedMetadataValue{key: {Block: block, TxNum: txnum, Metadata: metadata}})[key]
}

func (db *VersionedPersistence) SetStateMetadatas(ns driver2.Namespace, kvs map[driver2.PKey]driver2.VersionedMetadataValue) map[driver2.PKey]error {
	errs := make(map[driver2.PKey]error)
	vals := make(map[driver2.PKey][]any, len(kvs))
	for pkey, meta := range kvs {
		if val, err := db.metadataScanner.WriteValue(meta); err != nil {
			errs[pkey] = err
		} else {
			vals[pkey] = val
		}
	}
	if len(errs) > 0 {
		return errs
	}
	return db.UpsertStates(ns, db.metadataScanner.Columns(), vals)
}

func (db *VersionedPersistence) GetStateMetadata(namespace driver2.Namespace, key driver2.PKey) (driver2.Metadata, driver2.RawVersion, error) {
	var m []byte
	var meta map[string][]byte
	var kversion []byte

	query := fmt.Sprintf("SELECT metadata, kversion FROM %s WHERE ns = $1 AND pkey = $2", db.table)
	logger.Debug(query, namespace, key)

	row := db.readDB.QueryRow(query, namespace, key)
	if err := row.Scan(&m, &kversion); err != nil {
		if err == sql.ErrNoRows {
			logger.Debugf("not found: [%s:%s]", namespace, key)
			return meta, nil, nil
		}
		return meta, nil, fmt.Errorf("error querying db: %w", err)
	}
	meta, err := unmarshalMetadata(m)
	if err != nil {
		return meta, nil, fmt.Errorf("error decoding metadata: %w", err)
	}

	return meta, kversion, err
}

func (db *VersionedPersistence) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		ns TEXT NOT NULL,
		pkey BYTEA NOT NULL,
		val BYTEA NOT NULL DEFAULT '',
		kversion BYTEA DEFAULT '',
		metadata BYTEA NOT NULL DEFAULT '',
		fver INT NOT NULL DEFAULT 0,
		PRIMARY KEY (pkey, ns)
	);`, db.table))
}

func (db *VersionedPersistence) NewWriteTransaction() (driver.WriteTransaction, error) {
	txn, err := db.writeDB.Begin()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to begin transaction")
	}

	return NewWriteTransaction(txn, db), nil
}

type versionedPersistence interface {
	SetStateWithTx(tx *sql.Tx, ns driver2.Namespace, pkey string, value driver.VersionedValue) error
	DeleteStateWithTx(tx *sql.Tx, ns driver2.Namespace, key driver2.PKey) error
}

func NewWriteTransaction(txn *sql.Tx, db versionedPersistence) *WriteTransaction {
	return &WriteTransaction{txn: txn, db: db}
}

type WriteTransaction struct {
	txn *sql.Tx
	db  versionedPersistence
}

func (w *WriteTransaction) SetState(namespace driver2.Namespace, key driver2.PKey, value driver.VersionedValue) error {
	return w.db.SetStateWithTx(w.txn, namespace, key, value)
}

func (w *WriteTransaction) DeleteState(ns driver2.Namespace, key driver2.PKey) error {
	return w.db.DeleteStateWithTx(w.txn, ns, key)
}

func (w *WriteTransaction) Commit() error {
	if err := w.txn.Commit(); err != nil {
		return fmt.Errorf("could not commit transaction: %w", err)
	}
	w.txn = nil
	return nil
}

func (w *WriteTransaction) Discard() error {
	if err := w.txn.Rollback(); err != nil {
		logger.Infof("error rolling back (ignoring): %s", err.Error())
		return nil
	}
	w.txn = nil
	return nil
}

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
