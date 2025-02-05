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

type dbTransaction interface {
	Exec(query string, args ...any) (sql.Result, error)
}

type UnversionedPersistence struct {
	*common.BaseDB[*sql.Tx]
	writeDB WriteDB
	readDB  *sql.DB
	table   string

	errorWrapper driver.SQLErrorWrapper
	ci           Interpreter
}

func NewUnversionedPersistence(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *UnversionedPersistence {
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

func (db *UnversionedPersistence) DeleteStateWithTx(tx dbTransaction, ns driver2.Namespace, key driver2.PKey) error {
	if errs := db.DeleteStatesWithTx(tx, ns, key); errs != nil {
		return errs[key]
	}
	return nil
}

func (db *UnversionedPersistence) DeleteStates(namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	return db.DeleteStatesWithTx(db.Txn, namespace, keys...)
}

func (db *UnversionedPersistence) DeleteStatesWithTx(tx dbTransaction, namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	if db.IsTxnNil() {
		logger.Debugf("No ongoing transaction. Using db")
		tx = db.writeDB
	}

	if len(namespace) == 0 {
		return collections.RepeatValue(keys, errors.New("ns or key is empty"))
	}
	where, args := Where(db.hasKeys(namespace, keys))
	query := fmt.Sprintf("DELETE FROM %s %s", db.table, where)
	logger.Debug(query, args)
	_, err := tx.Exec(query, args...)
	if err != nil {
		errs := make(map[driver2.PKey]error)
		for _, key := range keys {
			errs[key] = errors.Wrapf(db.errorWrapper.WrapError(err), "could not deleteOp val for key [%s]", key)
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

func (db *UnversionedPersistence) SetState(ns driver2.Namespace, pkey driver2.PKey, value driver.UnversionedValue) error {
	return db.SetStateWithTx(db.Txn, ns, pkey, value)
}

func (db *UnversionedPersistence) SetStates(ns driver2.Namespace, kvs map[driver2.PKey]driver.UnversionedValue) map[driver2.PKey]error {
	return db.SetStatesWithTx(db.Txn, ns, kvs)
}

func (db *UnversionedPersistence) SetStateWithTx(tx dbTransaction, ns driver2.Namespace, pkey driver2.PKey, value driver.UnversionedValue) error {
	if errs := db.SetStatesWithTx(tx, ns, map[driver2.PKey]driver.UnversionedValue{pkey: value}); errs != nil {
		return errs[pkey]
	}
	return nil
}

func (db *UnversionedPersistence) SetStatesWithTx(tx dbTransaction, ns driver2.Namespace, kvs map[driver2.PKey]driver.UnversionedValue) map[driver2.PKey]error {
	if db.IsTxnNil() {
		logger.Info("No ongoing transaction. Using db")
		tx = db.writeDB
	}

	upserted := make(map[driver2.PKey]driver.UnversionedValue, len(kvs))
	deleted := make([]driver2.PKey, 0, len(kvs))
	for pkey, val := range kvs {
		// Get rawVal
		if len(val) == 0 {
			logger.Debugf("set key [%s:%s] to nil value, will be deleted instead", ns, pkey)
			deleted = append(deleted, pkey)
		} else {
			logger.Debugf("set state [%s,%s]", ns, pkey)
			// Overwrite rawVal
			upserted[pkey] = append([]byte(nil), val...)
		}
	}

	errs := make(map[driver2.PKey]error)
	if len(deleted) > 0 {
		collections.CopyMap(errs, db.DeleteStatesWithTx(tx, ns, deleted...))
	}
	if len(upserted) > 0 {
		collections.CopyMap(errs, db.upsertStatesWithTx(tx, ns, upserted))
	}
	return errs
}

func (db *UnversionedPersistence) upsertStatesWithTx(tx dbTransaction, ns driver2.Namespace, vals map[driver2.PKey]driver.UnversionedValue) map[driver2.PKey]error {
	query := fmt.Sprintf("INSERT INTO %s (ns, pkey, val) "+
		"VALUES %s "+
		"ON CONFLICT (ns, pkey) DO UPDATE "+
		"SET val=excluded.val",
		db.table,
		CreateParamsMatrix(3, len(vals), 1))

	args := make([]any, 0, 3*len(vals))
	for pkey, val := range vals {
		args = append(args, ns, pkey, val)
	}
	logger.Debug(query, args)
	if _, err := tx.Exec(query, args...); err != nil {
		return collections.RepeatValue(collections.Keys(vals), errors2.Wrapf(db.errorWrapper.WrapError(err), "could not upsert"))
	}
	return nil
}

func (db *UnversionedPersistence) Exec(query string, args ...any) (sql.Result, error) {
	return db.Txn.Exec(query, args...)
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
