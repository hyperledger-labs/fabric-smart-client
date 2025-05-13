/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"fmt"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	q "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/cond"
	"github.com/pkg/errors"
)

var logger = logging.MustGetLogger("view-sdk.db.driver.sql")

type dbTransaction interface {
	Exec(query string, args ...any) (sql.Result, error)
}

type KeyValueStore struct {
	*common.BaseDB[*sql.Tx]
	writeDB WriteDB
	readDB  *sql.DB
	table   string

	errorWrapper driver.SQLErrorWrapper
	ci           common2.CondInterpreter
}

func NewKeyValueStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci common2.CondInterpreter) *KeyValueStore {
	return &KeyValueStore{
		BaseDB:       common.NewBaseDB(func() (*sql.Tx, error) { return writeDB.Begin() }),
		readDB:       readDB,
		writeDB:      writeDB,
		table:        table,
		errorWrapper: errorWrapper,
		ci:           ci,
	}
}

func (db *KeyValueStore) GetStateRangeScanIterator(ns driver2.Namespace, startKey, endKey driver2.PKey) (collections.Iterator[*driver.UnversionedRead], error) {
	query, params := q.Select("pkey", "val").
		From(q.Table(db.table)).
		Where(cond.And(cond.Eq("ns", ns), cond.BetweenStrings(common2.FieldName("pkey"), startKey, endKey))).
		OrderBy(q.Asc(common2.FieldName("pkey"))).
		Format(db.ci, nil)

	logger.Debug(query, params)

	rows, err := db.readDB.Query(query, params...)
	if err != nil {
		return nil, errors2.Wrapf(err, "query error: %s", query)
	}

	return &readIterator{txs: rows}, nil
}

func (db *KeyValueStore) GetState(namespace driver2.Namespace, key driver2.PKey) (driver.UnversionedValue, error) {
	query, params := q.Select("val").
		From(q.Table(db.table)).
		Where(HasKeys(namespace, key)).
		Format(db.ci, nil)
	logger.Debug(query, params)

	return QueryUnique[driver.UnversionedValue](db.readDB, query, params...)
}

func (db *KeyValueStore) GetStateSetIterator(ns driver2.Namespace, keys ...driver2.PKey) (collections.Iterator[*driver.UnversionedRead], error) {
	if len(keys) == 0 {
		return collections.NewEmptyIterator[*driver.UnversionedRead](), nil
	}
	query, params := q.Select("pkey", "val").
		From(q.Table(db.table)).
		Where(HasKeys(ns, keys...)).
		Format(db.ci, nil)

	logger.Debug(query[:30] + "...")

	rows, err := db.readDB.Query(query, params...)
	if err != nil {
		return nil, errors2.Wrapf(err, "query error: %s", query)
	}

	return &readIterator{txs: rows}, nil
}

func HasKeys(ns driver2.Namespace, keys ...driver2.PKey) cond.Condition {
	return cond.And(cond.Eq("ns", ns), cond.In("pkey", keys...))
}

func (db *KeyValueStore) Close() error {
	logger.Info("closing database")

	// TODO: what to do with db.Txn if it's not nil?

	err := db.writeDB.Close()
	if err != nil {
		return errors2.Wrapf(err, "could not close DB")
	}

	return nil
}

func (db *KeyValueStore) DeleteState(ns driver2.Namespace, key driver2.PKey) error {
	return db.DeleteStateWithTx(db.Txn, ns, key)
}

func (db *KeyValueStore) DeleteStateWithTx(tx dbTransaction, ns driver2.Namespace, key driver2.PKey) error {
	if errs := db.DeleteStatesWithTx(tx, ns, key); errs != nil {
		return errs[key]
	}
	return nil
}

func (db *KeyValueStore) DeleteStates(namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	return db.DeleteStatesWithTx(db.Txn, namespace, keys...)
}

func (db *KeyValueStore) DeleteStatesWithTx(tx dbTransaction, namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	if db.IsTxnNil() {
		logger.Debug("No ongoing transaction. Using db")
		tx = db.writeDB
	}

	if len(namespace) == 0 {
		return collections.RepeatValue(keys, errors.New("ns or key is empty"))
	}
	query, params := q.DeleteFrom(db.table).
		Where(HasKeys(namespace, keys...)).
		Format(db.ci)

	logger.Debug(query, params)
	_, err := tx.Exec(query, params...)
	if err != nil {
		errs := make(map[driver2.PKey]error)
		for _, key := range keys {
			errs[key] = errors.Wrapf(db.errorWrapper.WrapError(err), "could not deleteOp val for key [%s]", key)
		}
		return errs
	}

	return nil
}

func (db *KeyValueStore) SetState(ns driver2.Namespace, pkey driver2.PKey, value driver.UnversionedValue) error {
	return db.SetStateWithTx(db.Txn, ns, pkey, value)
}

func (db *KeyValueStore) SetStates(ns driver2.Namespace, kvs map[driver2.PKey]driver.UnversionedValue) map[driver2.PKey]error {
	return db.SetStatesWithTx(db.Txn, ns, kvs)
}

func (db *KeyValueStore) SetStateWithTx(tx dbTransaction, ns driver2.Namespace, pkey driver2.PKey, value driver.UnversionedValue) error {
	if errs := db.SetStatesWithTx(tx, ns, map[driver2.PKey]driver.UnversionedValue{pkey: value}); errs != nil {
		return errs[pkey]
	}
	return nil
}

func (db *KeyValueStore) SetStatesWithTx(tx dbTransaction, ns driver2.Namespace, kvs map[driver2.PKey]driver.UnversionedValue) map[driver2.PKey]error {
	if db.IsTxnNil() {
		logger.Debug("No ongoing transaction. Using db")
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

func (db *KeyValueStore) upsertStatesWithTx(tx dbTransaction, ns driver2.Namespace, vals map[driver2.PKey]driver.UnversionedValue) map[driver2.PKey]error {
	rows := make([]common2.Tuple, 0, len(vals))
	for pkey, val := range vals {
		rows = append(rows, common2.Tuple{ns, pkey, val})
	}
	query, params := q.InsertInto(db.table).
		Fields("ns", "pkey", "val").
		Rows(rows).
		OnConflict([]common2.FieldName{"ns", "pkey"}, q.OverwriteValue("val")).
		Format()

	logger.Debug(query, params)
	if _, err := tx.Exec(query, params...); err != nil {
		return collections.RepeatValue(collections.Keys(vals), errors2.Wrapf(db.errorWrapper.WrapError(err), "could not upsert"))
	}
	return nil
}

func (db *KeyValueStore) Exec(query string, args ...any) (sql.Result, error) {
	return db.Txn.Exec(query, args...)
}

func (db *KeyValueStore) Stats() any {
	if db.readDB != nil {
		return db.readDB.Stats()
	}
	return nil
}

type readIterator struct {
	txs *sql.Rows
}

func (t *readIterator) Close() {
	utils.IgnoreErrorFunc(t.txs.Close)
}

func (t *readIterator) Next() (*driver.UnversionedRead, error) {
	if !t.txs.Next() {
		return nil, nil
	}
	var r driver.UnversionedRead
	err := t.txs.Scan(&r.Key, &r.Raw)
	return &r, err
}

func (db *KeyValueStore) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		ns TEXT NOT NULL,
		pkey TEXT NOT NULL,
		val BYTEA NOT NULL DEFAULT '',
		PRIMARY KEY (pkey, ns)
	);`, db.table))
}
