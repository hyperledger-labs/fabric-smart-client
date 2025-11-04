/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	q "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/common"
	cond2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/cond"
)

var logger = logging.MustGetLogger()

type dbTransaction interface {
	Exec(query string, args ...any) (sql.Result, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
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

func (db *KeyValueStore) GetStateRangeScanIterator(ctx context.Context, ns driver2.Namespace, startKey, endKey driver2.PKey) (iterators.Iterator[*driver.UnversionedRead], error) {
	query, params := q.Select().FieldsByName("pkey", "val").
		From(q.Table(db.table)).
		Where(cond2.And(cond2.Eq("ns", ns), cond2.BetweenBytes("pkey", []byte(startKey), []byte(endKey)))).
		OrderBy(q.Asc(common2.FieldName("pkey"))).
		Format(db.ci)

	logger.Debug(query, params)

	rows, err := db.readDB.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "query error: %s", query)
	}

	return NewIterator(rows, func(r *driver.UnversionedRead) error { return rows.Scan(&r.Key, &r.Raw) }), nil
}

func (db *KeyValueStore) GetState(ctx context.Context, namespace driver2.Namespace, key driver2.PKey) (driver.UnversionedValue, error) {
	query, params := q.Select().FieldsByName("val").
		From(q.Table(db.table)).
		Where(HasKeys(namespace, key)).
		Format(db.ci)

	return QueryUniqueContext[driver.UnversionedValue](ctx, db.readDB, query, params...)
}

func (db *KeyValueStore) GetStateSetIterator(ctx context.Context, ns driver2.Namespace, keys ...driver2.PKey) (iterators.Iterator[*driver.UnversionedRead], error) {
	if len(keys) == 0 {
		return collections.NewEmptyIterator[*driver.UnversionedRead](), nil
	}
	query, params := q.Select().FieldsByName("pkey", "val").
		From(q.Table(db.table)).
		Where(HasKeys(ns, keys...)).
		Format(db.ci)

	logger.Debug(query[:30] + "...")

	rows, err := db.readDB.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "query error: %s", query)
	}

	return NewIterator(rows, func(r *driver.UnversionedRead) error { return rows.Scan(&r.Key, &r.Raw) }), nil
}

func HasKeys(ns driver2.Namespace, keys ...driver2.PKey) cond2.Condition {
	kbytes := make([][]byte, len(keys))
	for i, k := range keys {
		kbytes[i] = []byte(k)
	}
	return cond2.And(cond2.Eq("ns", ns), cond2.In("pkey", kbytes...))
}

func (db *KeyValueStore) Close() error {
	logger.Info("closing database")
	err := db.writeDB.Close()
	if err != nil {
		return errors.Wrapf(err, "could not close DB")
	}

	return nil
}

func (db *KeyValueStore) DeleteState(ctx context.Context, ns driver2.Namespace, key driver2.PKey) error {
	return db.DeleteStateWithTx(ctx, db.Txn, ns, key)
}

func (db *KeyValueStore) DeleteStateWithTx(ctx context.Context, tx dbTransaction, ns driver2.Namespace, key driver2.PKey) error {
	if errs := db.DeleteStatesWithTx(ctx, tx, ns, key); errs != nil {
		return errs[key]
	}
	return nil
}

func (db *KeyValueStore) DeleteStates(ctx context.Context, namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	return db.DeleteStatesWithTx(ctx, db.Txn, namespace, keys...)
}

func (db *KeyValueStore) DeleteStatesWithTx(ctx context.Context, tx dbTransaction, namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	if db.IsTxnNil() {
		logger.Debug("no ongoing transaction. Using db")
		tx = db.writeDB
	}

	if len(namespace) == 0 {
		return collections.RepeatValue(keys, errors.New("ns or key is empty"))
	}
	query, params := q.DeleteFrom(db.table).
		Where(HasKeys(namespace, keys...)).
		Format(db.ci)

	logger.Debug(query, params)
	_, err := tx.ExecContext(ctx, query, params...)
	if err != nil {
		errs := make(map[driver2.PKey]error)
		for _, key := range keys {
			errs[key] = errors.Wrapf(db.errorWrapper.WrapError(err), "could not deleteOp val for key [%s]", key)
		}
		return errs
	}

	return nil
}

func (db *KeyValueStore) SetState(ctx context.Context, ns driver2.Namespace, pkey driver2.PKey, value driver.UnversionedValue) error {
	return db.SetStateWithTx(ctx, db.Txn, ns, pkey, value)
}

func (db *KeyValueStore) SetStates(ctx context.Context, ns driver2.Namespace, kvs map[driver2.PKey]driver.UnversionedValue) map[driver2.PKey]error {
	return db.SetStatesWithTx(ctx, db.Txn, ns, kvs)
}

func (db *KeyValueStore) SetStateWithTx(ctx context.Context, tx dbTransaction, ns driver2.Namespace, pkey driver2.PKey, value driver.UnversionedValue) error {
	if errs := db.SetStatesWithTx(ctx, tx, ns, map[driver2.PKey]driver.UnversionedValue{pkey: value}); errs != nil {
		return errs[pkey]
	}
	return nil
}

func (db *KeyValueStore) SetStatesWithTx(ctx context.Context, tx dbTransaction, ns driver2.Namespace, kvs map[driver2.PKey]driver.UnversionedValue) map[driver2.PKey]error {
	if db.IsTxnNil() {
		logger.Debug("No ongoing transaction. Using db")
		tx = db.writeDB
	}

	upserted := make(map[driver2.PKey]driver.UnversionedValue, len(kvs))
	deleted := make([]driver2.PKey, 0, len(kvs))
	for pkey, val := range kvs {
		// Get rawVal
		if len(val) == 0 {
			logger.DebugfContext(ctx, "set key [%s:%s] to nil value, will be deleted instead", ns, pkey)
			deleted = append(deleted, pkey)
		} else {
			logger.DebugfContext(ctx, "set state [%s,%s]", ns, pkey)
			// Overwrite rawVal
			upserted[pkey] = append([]byte(nil), val...)
		}
	}

	errs := make(map[driver2.PKey]error)
	if len(deleted) > 0 {
		collections.CopyMap(errs, db.DeleteStatesWithTx(ctx, tx, ns, deleted...))
	}
	if len(upserted) > 0 {
		collections.CopyMap(errs, db.upsertStatesWithTx(ctx, tx, ns, upserted))
	}
	return errs
}

func (db *KeyValueStore) upsertStatesWithTx(ctx context.Context, tx dbTransaction, ns driver2.Namespace, vals map[driver2.PKey]driver.UnversionedValue) map[driver2.PKey]error {
	rows := make([]common2.Tuple, 0, len(vals))
	for pkey, val := range vals {
		rows = append(rows, common2.Tuple{ns, []byte(pkey), val})
	}
	query, params := q.InsertInto(db.table).
		Fields("ns", "pkey", "val").
		Rows(rows).
		OnConflict([]common2.FieldName{"ns", "pkey"}, q.OverwriteValue("val")).
		Format()

	logger.Debug(query, params)
	if _, err := tx.ExecContext(ctx, query, params...); err != nil {
		return collections.RepeatValue(collections.Keys(vals), errors.Wrapf(db.errorWrapper.WrapError(err), "could not upsert"))
	}
	return nil
}

func (db *KeyValueStore) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return db.Txn.ExecContext(ctx, query, args...)
}

func (db *KeyValueStore) Stats() any {
	if db.readDB != nil {
		return db.readDB.Stats()
	}
	return nil
}

func (db *KeyValueStore) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		ns TEXT NOT NULL,
		pkey BYTEA NOT NULL,
		val BYTEA NOT NULL DEFAULT '',
		PRIMARY KEY (pkey, ns)
	);`, db.table))
}
