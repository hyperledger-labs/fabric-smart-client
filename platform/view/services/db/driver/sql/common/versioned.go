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

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	errors2 "github.com/pkg/errors"
)

func NewVersionedReadScanner() *versionedReadScanner { return &versionedReadScanner{} }

func NewVersionedValueScanner() *versionedValueScanner { return &versionedValueScanner{} }

type versionedReadScanner struct{}

func (s *versionedReadScanner) Columns() []string { return []string{"pkey", "block", "txnum", "val"} }

func (s *versionedReadScanner) ReadValue(txs scannable) (driver.VersionedRead, error) {
	var r driver.VersionedRead
	err := txs.Scan(&r.Key, &r.Block, &r.TxNum, &r.Raw)
	return r, err
}

type versionedValueScanner struct{}

func (s *versionedValueScanner) Columns() []string { return []string{"val", "block", "txnum"} }

func (s *versionedValueScanner) ReadValue(txs scannable) (driver.VersionedValue, error) {
	var r driver.VersionedValue
	err := txs.Scan(&r.Raw, &r.Block, &r.TxNum)
	return r, err
}

func (s *versionedValueScanner) WriteValue(value driver.VersionedValue) []any {
	return []any{value.Raw, value.Block, value.TxNum}
}

type basePersistence[V any, R any] interface {
	driver.BasePersistence[driver.VersionedValue, driver.VersionedRead]
	Exists(namespace driver2.Namespace, key driver2.PKey) (bool, error)
	Exec(query string, args ...any) (sql.Result, error)
	SetStateWithTx(tx *sql.Tx, namespace driver2.Namespace, key string, value driver.VersionedValue) error
	DeleteStateWithTx(tx *sql.Tx, ns driver2.Namespace, key driver2.PKey) error
}

type VersionedPersistence struct {
	basePersistence[driver.VersionedValue, driver.VersionedRead]
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      *sql.DB
}

func NewVersionedPersistence(base basePersistence[driver.VersionedValue, driver.VersionedRead], table string, errorWrapper driver.SQLErrorWrapper, readDB *sql.DB, writeDB *sql.DB) *VersionedPersistence {
	return &VersionedPersistence{basePersistence: base, table: table, errorWrapper: errorWrapper, readDB: readDB, writeDB: writeDB}
}

func NewVersioned(readDB *sql.DB, writeDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *VersionedPersistence {
	base := NewBasePersistence[driver.VersionedValue, driver.VersionedRead](writeDB, readDB, table, &versionedReadScanner{}, &versionedValueScanner{}, errorWrapper, ci, writeDB.Begin)
	return NewVersionedPersistence(base, base.table, base.errorWrapper, base.readDB, base.writeDB)
}

func (db *VersionedPersistence) SetStateMetadata(ns driver2.Namespace, key driver2.PKey, metadata driver2.Metadata, block driver2.BlockNum, txnum driver2.TxNum) error {
	if ns == "" || key == "" {
		return errors.New("ns or key is empty")
	}
	if len(metadata) == 0 {
		return nil
	}
	m, err := marshallMetadata(metadata)
	if err != nil {
		return fmt.Errorf("error encoding metadata: %w", err)
	}

	exists, err := db.Exists(ns, key)
	if err != nil {
		return err
	}
	if exists {
		// Note: for consistency with badger we also update the block and txnum
		query := fmt.Sprintf("UPDATE %s SET metadata = $1, block = $2, txnum = $3 WHERE ns = $4 AND pkey = $5", db.table)
		logger.Debug(query, len(m), block, txnum, ns, key)
		_, err = db.Exec(query, m, block, txnum, ns, key)
		if err != nil {
			return errors2.Wrapf(db.errorWrapper.WrapError(err), "could not set metadata for key [%s]", key)
		}
	} else {
		logger.Warnf("storing metadata without existing value at [%s]", key)
		query := fmt.Sprintf("INSERT INTO %s (ns, pkey, metadata, block, txnum) VALUES ($1, $2, $3, $4, $5)", db.table)
		logger.Debug(query, ns, key, len(m), block, txnum)
		_, err = db.Exec(query, ns, key, m, block, txnum)
		if err != nil {
			return errors2.Wrapf(db.errorWrapper.WrapError(err), "could not set metadata for key [%s]", key)
		}
	}

	return nil
}

func (db *VersionedPersistence) SetStateMetadatas(ns driver2.Namespace, kvs map[driver2.PKey]driver2.Metadata, block driver2.BlockNum, txnum driver2.TxNum) map[driver2.PKey]error {
	errs := make(map[driver2.PKey]error)
	for pkey, value := range kvs {
		if err := db.SetStateMetadata(ns, pkey, value, block, txnum); err != nil {
			errs[pkey] = err
		}
	}
	return errs
}

func (db *VersionedPersistence) GetStateMetadata(ns driver2.Namespace, key driver2.PKey) (driver2.Metadata, driver2.BlockNum, driver2.TxNum, error) {
	var m []byte
	var meta map[string][]byte
	var block, txnum uint64

	query := fmt.Sprintf("SELECT metadata, block, txnum FROM %s WHERE ns = $1 AND pkey = $2", db.table)
	logger.Debug(query, ns, key)

	row := db.readDB.QueryRow(query, ns, key)
	if err := row.Scan(&m, &block, &txnum); err != nil {
		if err == sql.ErrNoRows {
			logger.Debugf("not found: [%s:%s]", ns, key)
			return meta, block, txnum, nil
		}
		return meta, block, txnum, fmt.Errorf("error querying db: %w", err)
	}
	meta, err := unmarshalMetadata(m)
	if err != nil {
		return meta, block, txnum, fmt.Errorf("error decoding metadata: %w", err)
	}

	return meta, block, txnum, err
}

func (db *VersionedPersistence) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		ns TEXT NOT NULL,
		pkey BYTEA NOT NULL,
		block BIGINT NOT NULL DEFAULT 0,
		txnum BIGINT NOT NULL DEFAULT 0,
		val BYTEA NOT NULL DEFAULT '',
		metadata BYTEA NOT NULL DEFAULT '',
		version INT NOT NULL DEFAULT 0,
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
