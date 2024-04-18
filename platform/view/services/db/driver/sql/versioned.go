/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	"bytes"
	"database/sql"
	"encoding/gob"
	"errors"
	"fmt"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	errors2 "github.com/pkg/errors"
)

type Persistence struct {
	base
	errorWrapper driver.SQLErrorWrapper
}

func NewPersistence(readDB *sql.DB, writeDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper) *Persistence {
	return &Persistence{
		base: base{
			readDB:  readDB,
			writeDB: writeDB,
			table:   table,
		},
		errorWrapper: errorWrapper,
	}
}

func (db *Persistence) SetState(ns string, key string, val []byte, block, txnum uint64) error {
	return db.setStateWithTx(nil, ns, key, val, block, txnum)
}

func (db *Persistence) setStateWithTx(tx *sql.Tx, ns, key string, val []byte, block, txnum uint64) error {
	if len(val) == 0 {
		logger.Warnf("set key [%s:%d:%d] to nil value, will be deleted instead", key, block, txnum)
		return db.DeleteState(ns, key)
	}
	if tx == nil {
		tx = db.txn
	}
	if tx == nil {
		panic("programming error, writing without ongoing update")
	}
	logger.Debugf("set state [%s,%s]", ns, key)

	val = append([]byte(nil), val...)

	// Note: INSERT ON CONFLICT works for postgres and sqlite, but there is no single-shot upsert in the sql standard.
	// Here we sacrifice a bit of performance to remain compatible with other databases.
	exists, err := db.exists(tx, ns, key)
	if err != nil {
		return err
	}
	if exists {
		query := fmt.Sprintf("UPDATE %s SET block = $1, txnum = $2, val = $3 WHERE ns = $4 AND pkey = $5", db.table)
		logger.Debug(query, block, txnum, len(val), ns, key)

		_, err := tx.Exec(query, block, txnum, val, ns, key)
		if err != nil {
			return errors2.Wrapf(db.errorWrapper.WrapError(err), "could not set val for key [%s]", key)
		}
	} else {
		query := fmt.Sprintf("INSERT INTO %s (ns, pkey, block, txnum, val) VALUES ($1, $2, $3, $4, $5)", db.table)
		logger.Debug(query, ns, key, block, txnum, len(val))

		_, err := tx.Exec(query, ns, key, block, txnum, val)
		if err != nil {
			return errors2.Wrapf(db.errorWrapper.WrapError(err), "could not insert [%s]", key)
		}
	}

	return nil
}

func (db *Persistence) GetState(ns, key string) ([]byte, uint64, uint64, error) {
	var val []byte
	var block, txnum uint64

	query := fmt.Sprintf("SELECT val, block, txnum FROM %s WHERE ns = $1 AND pkey = $2", db.table)
	logger.Debug(query, ns, key)

	row := db.readDB.QueryRow(query, ns, key)
	if err := row.Scan(&val, &block, &txnum); err != nil {
		if err == sql.ErrNoRows {
			logger.Debugf("not found: [%s:%s]", ns, key)
			return val, block, txnum, nil
		}
		return val, block, txnum, fmt.Errorf("error querying db: %w", err)
	}

	return val, block, txnum, nil
}

func (db *Persistence) SetStateMetadata(ns, key string, metadata map[string][]byte, block, txnum uint64) error {
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

func (db *Persistence) GetStateMetadata(ns, key string) (map[string][]byte, uint64, uint64, error) {
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

func (db *Persistence) GetStateSetIterator(ns string, keys ...string) (driver.VersionedResultsIterator, error) {
	if len(keys) == 0 {
		return &EmptyVersionedIterator{}, nil
	}
	query := fmt.Sprintf("SELECT pkey, block, txnum, val FROM %s WHERE ns = ? AND pkey = ANY(%s);", db.table, "?"+strings.Repeat(",?", len(keys)-1))
	logger.Debug(query, ns, keys)

	rows, err := db.readDB.Query(query, append([]any{ns}, castAny(keys)...)...)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}

	return &VersionedReadIterator{
		txs: rows,
	}, nil
}

func (db *Persistence) GetStateRangeScanIterator(ns string, startKey string, endKey string) (driver.VersionedResultsIterator, error) {
	where, args := rangeWhere(ns, startKey, endKey)
	query := fmt.Sprintf("SELECT pkey, block, txnum, val FROM %s WHERE ns = $1 ", db.table) + where + " ORDER BY pkey;"
	logger.Debug(query, ns, startKey, endKey)

	rows, err := db.readDB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}

	return &VersionedReadIterator{
		txs: rows,
	}, nil
}

type EmptyVersionedIterator struct{}

func (t *EmptyVersionedIterator) Close() {}

func (t *EmptyVersionedIterator) Next() (*driver.VersionedRead, error) { return nil, nil }

type VersionedReadIterator struct {
	txs *sql.Rows
}

func (t *VersionedReadIterator) Close() {
	t.txs.Close()
}

func (t *VersionedReadIterator) Next() (*driver.VersionedRead, error) {
	var r driver.VersionedRead
	if !t.txs.Next() {
		return nil, nil
	}
	err := t.txs.Scan(&r.Key, &r.Block, &r.IndexInBlock, &r.Raw)

	return &r, err
}

func (db *Persistence) CreateSchema() error {
	query := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		ns TEXT NOT NULL,
		pkey BYTEA NOT NULL,
		block BIGINT NOT NULL DEFAULT 0,
		txnum BIGINT NOT NULL DEFAULT 0,
		val BYTEA NOT NULL DEFAULT '',
		metadata BYTEA NOT NULL DEFAULT '',
		version INT NOT NULL DEFAULT 0,
		PRIMARY KEY (pkey, ns)
	);`, db.table)

	logger.Debug(query)
	if _, err := db.writeDB.Exec(query); err != nil {
		return fmt.Errorf("can't create table: %w", err)
	}
	return nil
}

func (db *Persistence) NewWriteTransaction() (driver.WriteTransaction, error) {
	txn, err := db.writeDB.Begin()
	if err != nil {
		return nil, err
	}

	return &WriteTransaction{
		txn: txn,
		db:  db,
	}, nil
}

type WriteTransaction struct {
	txn *sql.Tx
	db  *Persistence
}

func (w *WriteTransaction) SetState(namespace, key string, value []byte, block, txnum uint64) error {
	if w.txn == nil {
		panic("programming error, writing without ongoing update")
	}

	return w.db.setStateWithTx(w.txn, namespace, key, value, block, txnum)
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
