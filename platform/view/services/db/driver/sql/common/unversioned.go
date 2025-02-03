/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"fmt"
	"time"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/pkg/errors"
)

var logger = logging.MustGetLogger("view-sdk.db.driver.sql")

type UnversionedPersistence struct {
	*common.BaseDB[*sql.Tx]
	writeDB *sql.DB
	readDB  *sql.DB
	table   string

	errorWrapper driver.SQLErrorWrapper
	ci           Interpreter
}

func NewUnversionedPersistence(writeDB *sql.DB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *UnversionedPersistence {
	return &UnversionedPersistence{
		BaseDB:       common.NewBaseDB(func() (*sql.Tx, error) { return writeDB.Begin() }),
		readDB:       readDB,
		writeDB:      writeDB,
		table:        table,
		errorWrapper: errorWrapper,
		ci:           ci,
	}
}

func (db *UnversionedPersistence) GetStateRangeScanIterator(ns driver2.Namespace, startKey, endKey driver2.PKey) (collections.Iterator[*driver.UnversionedRead], error) {
	where, args := Where(db.ci.And(
		db.ci.Cmp("ns", "=", ns),
		db.ci.BetweenStrings("pkey", startKey, endKey),
	))
	query := fmt.Sprintf("SELECT pkey, val FROM %s %s ORDER BY pkey;", db.table, where)
	logger.Debug(query, ns, startKey, endKey)

	rows, err := db.readDB.Query(query, args...)
	if err != nil {
		return nil, errors2.Wrapf(err, "query error: %s", query)
	}

	return &readIterator{txs: rows}, nil
}

func (db *UnversionedPersistence) GetState(namespace driver2.Namespace, key driver2.PKey) (driver.UnversionedValue, error) {
	where, args := Where(db.hasKey(namespace, key))
	query := fmt.Sprintf("SELECT val FROM %s %s", db.table, where)
	logger.Debug(query, args)

	return QueryUnique[driver.UnversionedValue](db.readDB, query, args...)
}

func (db *UnversionedPersistence) SetState(ns driver2.Namespace, pkey driver2.PKey, value driver.UnversionedValue) error {
	return db.SetStateWithTx(db.Txn, ns, pkey, value)
}

func (db *UnversionedPersistence) SetStates(ns driver2.Namespace, kvs map[driver2.PKey]driver.UnversionedValue) map[driver2.PKey]error {
	errs := make(map[driver2.PKey]error)
	for pkey, value := range kvs {
		if err := db.SetState(ns, pkey, value); err != nil {
			errs[pkey] = err
		}
	}
	return errs
}

func (db *UnversionedPersistence) GetStateSetIterator(ns driver2.Namespace, keys ...driver2.PKey) (collections.Iterator[*driver.UnversionedRead], error) {
	if len(keys) == 0 {
		return collections.NewEmptyIterator[*driver.UnversionedRead](), nil
	}
	where, args := Where(db.ci.And(
		db.ci.Cmp("ns", "=", ns),
		db.ci.InStrings("pkey", keys),
	))
	query := fmt.Sprintf("SELECT pkey, val FROM %s %s", db.table, where)
	logger.Debug(query[:30] + "...")

	rows, err := db.readDB.Query(query, args...)
	if err != nil {
		return nil, errors2.Wrapf(err, "query error: %s", query)
	}

	return &readIterator{txs: rows}, nil
}

func (db *UnversionedPersistence) Close() error {
	logger.Info("closing database")

	// TODO: what to do with db.Txn if it's not nil?

	err := db.writeDB.Close()
	if err != nil {
		return errors2.Wrapf(err, "could not close DB")
	}

	return nil
}

func (db *UnversionedPersistence) DeleteState(ns driver2.Namespace, key driver2.PKey) error {
	return db.DeleteStateWithTx(db.Txn, ns, key)
}

func (db *UnversionedPersistence) DeleteStateWithTx(tx *sql.Tx, ns driver2.Namespace, key driver2.PKey) error {
	if errs := db.DeleteStatesWithTx(tx, ns, key); errs != nil {
		return errs[key]
	}
	return nil
}

func (db *UnversionedPersistence) DeleteStates(namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	return db.DeleteStatesWithTx(db.Txn, namespace, keys...)
}

func (db *UnversionedPersistence) DeleteStatesWithTx(tx *sql.Tx, namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	if tx == nil {
		panic("programming error, writing without ongoing update")
	}
	if namespace == "" {
		return collections.RepeatValue(keys, errors.New("ns or key is empty"))
	}
	where, args := Where(db.hasKeys(namespace, keys))
	query := fmt.Sprintf("DELETE FROM %s %s", db.table, where)
	logger.Debug(query, args)
	_, err := tx.Exec(query, args...)
	if err != nil {
		errs := make(map[driver2.PKey]error)
		for _, key := range keys {
			errs[key] = errors.Wrapf(db.errorWrapper.WrapError(err), "could not delete val for key [%s]", key)
		}
		return errs
	}

	return nil
}

func (db *UnversionedPersistence) hasKeys(ns driver2.Namespace, pkeys []driver2.PKey) Condition {
	return db.ci.And(
		db.ci.Cmp("ns", "=", ns),
		db.ci.InStrings("pkey", pkeys),
	)
}

func (db *UnversionedPersistence) SetStateWithTx(tx *sql.Tx, ns driver2.Namespace, pkey driver2.PKey, val driver.UnversionedValue) error {
	if tx == nil {
		panic("programming error, writing without ongoing update")
	}
	if len(val) == 0 {
		logger.Debugf("set key [%s:%s] to nil value, will be deleted instead", ns, pkey)
		return db.DeleteState(ns, pkey)
	}

	logger.Debugf("set state [%s,%s]", ns, pkey)

	val = append([]byte(nil), val...)

	// Portable upsert
	exists, err := db.exists(tx, ns, pkey)
	if err != nil {
		return err
	}

	if exists {

		cond := db.hasKey(ns, pkey)
		where, args := cond.ToString(CopyPtr(2)), cond.Params()
		query := fmt.Sprintf("UPDATE %s SET val=$1 WHERE %s", db.table, where)
		logger.Debug(query, val, args)

		_, err := tx.Exec(query, append([]any{val}, args...)...)
		if err != nil {
			return errors2.Wrapf(db.errorWrapper.WrapError(err), "could not set val for key [%s]", pkey)
		}
	} else {
		query := fmt.Sprintf("INSERT INTO %s (ns, pkey, val) VALUES %s", db.table, GenerateParamSet(1, 1, 3))
		logger.Debug(query, ns, pkey, len(val))

		_, err := tx.Exec(query, ns, pkey, val)
		if err != nil {
			return errors2.Wrapf(db.errorWrapper.WrapError(err), "could not insert [%s]", pkey)
		}
	}
	logger.Debugf("set state [%s,%s], done", ns, pkey)

	return nil
}

func (db *UnversionedPersistence) Exec(query string, args ...any) (sql.Result, error) {
	return db.Txn.Exec(query, args...)
}

func (db *UnversionedPersistence) Exists(ns driver2.Namespace, key driver2.PKey) (bool, error) {
	if db.Txn == nil {
		panic("programming error, writing without ongoing update")
	}
	return db.exists(db.Txn, ns, key)
}

func (db *UnversionedPersistence) exists(tx *sql.Tx, ns driver2.Namespace, key driver2.PKey) (bool, error) {
	var pkey driver2.PKey
	where, args := Where(db.hasKey(ns, key))
	query := fmt.Sprintf("SELECT pkey FROM %s %s", db.table, where)
	logger.Debug(query, args)
	err := tx.QueryRow(query, args...).Scan(&pkey)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, errors2.Wrapf(err, "cannot check if key exists: %s", key)
	}
	return true, nil
}

func (db *UnversionedPersistence) hasKey(ns driver2.Namespace, pkey string) Condition {
	return db.ci.And(
		db.ci.Cmp("ns", "=", ns),
		db.ci.Cmp("pkey", "=", pkey),
	)
}

func (db *UnversionedPersistence) Stats() any {
	if db.readDB != nil {
		return db.readDB.Stats()
	}
	return nil
}

type readIterator struct {
	txs *sql.Rows
}

func (t *readIterator) Close() {
	t.txs.Close()
}

func (t *readIterator) Next() (*driver.UnversionedRead, error) {
	if !t.txs.Next() {
		return nil, nil
	}
	var r driver.UnversionedRead
	err := t.txs.Scan(&r.Key, &r.Raw)
	return &r, err
}

func (db *UnversionedPersistence) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		ns TEXT NOT NULL,
		pkey TEXT NOT NULL,
		val BYTEA NOT NULL DEFAULT '',
		PRIMARY KEY (pkey, ns)
	);`, db.table))
}

func (db *UnversionedPersistence) NewWriteTransaction() (driver.UnversionedWriteTransaction, error) {
	txn, err := db.writeDB.Begin()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to begin transaction")
	}

	return NewWriteTransaction(txn, db), nil
}

type versionedPersistence interface {
	SetStateWithTx(tx *sql.Tx, ns driver2.Namespace, pkey string, value driver.UnversionedValue) error
	DeleteStateWithTx(tx *sql.Tx, ns driver2.Namespace, key driver2.PKey) error
}

func NewWriteTransaction(txn *sql.Tx, db versionedPersistence) *WriteTransaction {
	return &WriteTransaction{txn: txn, db: db}
}

type WriteTransaction struct {
	txn *sql.Tx
	db  versionedPersistence
}

func (w *WriteTransaction) SetState(namespace driver2.Namespace, key driver2.PKey, value driver.UnversionedValue) error {
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

type SQLDriverType string

type Opts struct {
	Driver          SQLDriverType
	DataSource      string
	TablePrefix     string
	SkipCreateTable bool
	SkipPragmas     bool
	MaxOpenConns    int
	MaxIdleConns    *int
	MaxIdleTime     *time.Duration
}
