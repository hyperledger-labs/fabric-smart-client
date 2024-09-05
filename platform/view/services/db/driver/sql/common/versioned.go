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

type VersionedPersistence struct {
	basePersistence[driver.VersionedValue, driver.VersionedRead]
	errorWrapper driver.SQLErrorWrapper
}

func NewVersionedPersistence(readDB *sql.DB, writeDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *VersionedPersistence {
	return &VersionedPersistence{
		basePersistence: basePersistence[driver.VersionedValue, driver.VersionedRead]{
			readDB:       readDB,
			writeDB:      writeDB,
			table:        table,
			readScanner:  &versionedReadScanner{},
			valueScanner: &versionedValueScanner{},
			errorWrapper: errorWrapper,
			ci:           ci,
		},
		errorWrapper: errorWrapper,
	}
}

func (db *VersionedPersistence) SetStateMetadata(ns, key string, metadata map[string][]byte, block, txnum uint64) error {
	if db.txn == nil {
		panic("programming error, writing without ongoing update")
	}
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

	exists, err := db.exists(db.txn, ns, key)
	if err != nil {
		return err
	}
	if exists {
		// Note: for consistency with badger we also update the block and txnum
		query := fmt.Sprintf("UPDATE %s SET metadata = $1, block = $2, txnum = $3 WHERE ns = $4 AND pkey = $5", db.table)
		logger.Debug(query, len(m), block, txnum, ns, key)
		_, err = db.txn.Exec(query, m, block, txnum, ns, key)
		if err != nil {
			return errors2.Wrapf(db.errorWrapper.WrapError(err), "could not set metadata for key [%s]", key)
		}
	} else {
		logger.Warnf("storing metadata without existing value at [%s]", key)
		query := fmt.Sprintf("INSERT INTO %s (ns, pkey, metadata, block, txnum) VALUES ($1, $2, $3, $4, $5)", db.table)
		logger.Debug(query, ns, key, len(m), block, txnum)
		_, err = db.txn.Exec(query, ns, key, m, block, txnum)
		if err != nil {
			return errors2.Wrapf(db.errorWrapper.WrapError(err), "could not set metadata for key [%s]", key)
		}
	}

	return nil
}

func (db *VersionedPersistence) GetStateMetadata(ns, key string) (map[string][]byte, uint64, uint64, error) {
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
	return db.createSchema(fmt.Sprintf(`
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

	return &WriteTransaction{
		txn: txn,
		db:  db,
	}, nil
}

type WriteTransaction struct {
	txn *sql.Tx
	db  *VersionedPersistence
}

func (w *WriteTransaction) SetState(namespace driver2.Namespace, key string, value driver.VersionedValue) error {
	return w.db.setState(w.txn, namespace, key, value)
}

func (w *WriteTransaction) DeleteState(ns driver2.Namespace, key string) error {
	if ns == "" || key == "" {
		return errors.New("ns or key is empty")
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE ns = $1 AND pkey = $2", w.db.table)
	logger.Debug(query, ns, key)
	_, err := w.txn.Exec(query, ns, key)
	if err != nil {
		return errors2.Wrapf(w.db.errorWrapper.WrapError(err), "could not delete val for key [%s]", key)
	}

	return nil
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
