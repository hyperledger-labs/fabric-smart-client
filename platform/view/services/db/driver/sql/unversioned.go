/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/pkg/errors"
)

type Unversioned struct {
	base
	errorWrapper driver.SQLErrorWrapper
}

func NewUnversioned(readDB *sql.DB, writeDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper) *Unversioned {
	return &Unversioned{
		base: base{
			writeDB: writeDB,
			readDB:  readDB,
			table:   table,
		},
		errorWrapper: errorWrapper,
	}
}

func (db *Unversioned) SetState(ns, key string, val []byte) error {
	if len(val) == 0 {
		logger.Warnf("set key [%s:%s] to nil value, will be deleted instead", ns, key)
		return db.DeleteState(ns, key)
	}
	if db.txn == nil {
		panic("programming error, writing without ongoing update")
	}
	logger.Debugf("set state [%s,%s]", ns, key)

	val = append([]byte(nil), val...)

	// Portable upsert
	exists, err := db.exists(db.txn, ns, key)
	if err != nil {
		return err
	}
	if exists {
		query := fmt.Sprintf("UPDATE %s SET val = $1 WHERE ns = $2 AND pkey = $3", db.table)
		logger.Debug(query, len(val), ns, key)

		_, err := db.txn.Exec(query, val, ns, key)
		if err != nil {
			return errors.Wrapf(db.errorWrapper.WrapError(err), "could not set val for key [%s]", key)
		}
	} else {
		query := fmt.Sprintf("INSERT INTO %s (ns, pkey, val) VALUES ($1, $2, $3)", db.table)
		logger.Debug(query, ns, key, len(val))

		_, err := db.txn.Exec(query, ns, key, val)
		if err != nil {
			return errors.Wrapf(db.errorWrapper.WrapError(err), "could not insert [%s]", key)
		}
	}

	return nil
}

func (db *Unversioned) GetState(ns, key string) ([]byte, error) {
	var val []byte

	query := fmt.Sprintf("SELECT val FROM %s WHERE ns = $1 AND pkey = $2", db.table)
	logger.Debug(query, ns, key)

	row := db.readDB.QueryRow(query, ns, key)
	if err := row.Scan(&val); err != nil {
		if err == sql.ErrNoRows {
			logger.Debugf("not found: [%s:%s]", ns, key)
			return val, nil
		}
		return val, fmt.Errorf("error querying db: %w", err)
	}

	return val, nil
}

func (db *Unversioned) GetStateRangeScanIterator(ns string, startKey string, endKey string) (driver.ResultsIterator, error) {
	where, args := rangeWhere(ns, startKey, endKey)
	query := fmt.Sprintf("SELECT pkey, val FROM %s WHERE ns = $1 %s ORDER BY pkey;", db.table, where)
	logger.Debug(query, ns, startKey, endKey)

	rows, err := db.readDB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}

	return &UnversionedReadIterator{
		txs: rows,
	}, nil
}

func rangeWhere(ns, startKey, endKey string) (string, []interface{}) {
	where := ""
	args := []interface{}{ns}

	// To match badger behavior, we don't include the endKey
	if startKey != "" && endKey != "" {
		where = "AND pkey >= $2 AND pkey < $3"
		args = []interface{}{ns, startKey, endKey}
	} else if startKey != "" {
		where = "AND pkey >= $2"
		args = []interface{}{ns, startKey}
	} else if endKey != "" {
		where = "AND pkey < $2"
		args = []interface{}{ns, endKey}
	}
	return where, args
}

type UnversionedReadIterator struct {
	txs *sql.Rows
}

func (t *UnversionedReadIterator) Close() {
	t.txs.Close()
}

func (t *UnversionedReadIterator) Next() (*driver.Read, error) {
	var r driver.Read
	if !t.txs.Next() {
		return nil, nil
	}
	err := t.txs.Scan(&r.Key, &r.Raw)

	return &r, err
}

func (db *Unversioned) CreateSchema() error {
	query := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		ns TEXT NOT NULL,
		pkey BYTEA NOT NULL,
		val BYTEA NOT NULL DEFAULT '',
		PRIMARY KEY (pkey, ns)
	);`, db.table)

	logger.Debug(query)
	if _, err := db.writeDB.Exec(query); err != nil {
		return fmt.Errorf("can't create table: %w", err)
	}
	return nil
}
