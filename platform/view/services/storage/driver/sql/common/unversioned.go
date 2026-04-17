/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"database/sql"
	"fmt"

	sq "github.com/Masterminds/squirrel"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
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
	sb           sq.StatementBuilderType
}

func NewKeyValueStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ph sq.PlaceholderFormat) *KeyValueStore {
	return &KeyValueStore{
		BaseDB:       common.NewBaseDB(func() (*sql.Tx, error) { return writeDB.Begin() }),
		readDB:       readDB,
		writeDB:      writeDB,
		table:        table,
		errorWrapper: errorWrapper,
		sb:           sq.StatementBuilder.PlaceholderFormat(ph),
	}
}

func hasKeysCondition(ns driver2.Namespace, keys ...driver2.PKey) sq.Sqlizer {
	if len(keys) == 1 {
		return sq.And{sq.Eq{"ns": ns}, sq.Eq{"pkey": []byte(keys[0])}}
	}
	pkeys := make([][]byte, len(keys))
	for i, k := range keys {
		pkeys[i] = []byte(k)
	}
	return sq.And{sq.Eq{"ns": ns}, sq.Eq{"pkey": pkeys}}
}

func (db *KeyValueStore) GetStateRangeScanIterator(ctx context.Context, ns driver2.Namespace, startKey, endKey driver2.PKey) (iterators.Iterator[*driver.UnversionedRead], error) {
	conds := sq.And{sq.Eq{"ns": ns}}
	if len(startKey) != 0 {
		conds = append(conds, sq.GtOrEq{"pkey": []byte(startKey)})
	}
	if len(endKey) != 0 {
		conds = append(conds, sq.Lt{"pkey": []byte(endKey)})
	}
	query, params, err := db.sb.Select("pkey", "val").
		From(db.table).
		Where(conds).
		OrderBy("pkey ASC").
		ToSql()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build query")
	}
	logger.Debug(query, params)

	rows, err := db.readDB.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "query error: %s", query)
	}

	return NewIterator(rows, func(r *driver.UnversionedRead) error { return rows.Scan(&r.Key, &r.Raw) }), nil
}

func (db *KeyValueStore) GetState(ctx context.Context, namespace driver2.Namespace, key driver2.PKey) (driver.UnversionedValue, error) {
	query, params, err := db.sb.Select("val").
		From(db.table).
		Where(hasKeysCondition(namespace, key)).
		ToSql()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build query")
	}
	return QueryUniqueContext[driver.UnversionedValue](ctx, db.readDB, query, params...)
}

func (db *KeyValueStore) GetStateSetIterator(ctx context.Context, ns driver2.Namespace, keys ...driver2.PKey) (iterators.Iterator[*driver.UnversionedRead], error) {
	if len(keys) == 0 {
		return collections.NewEmptyIterator[*driver.UnversionedRead](), nil
	}
	query, params, err := db.sb.Select("pkey", "val").
		From(db.table).
		Where(hasKeysCondition(ns, keys...)).
		ToSql()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build query")
	}
	logger.Debug(query[:30] + "...")

	rows, err := db.readDB.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "query error: %s", query)
	}

	return NewIterator(rows, func(r *driver.UnversionedRead) error { return rows.Scan(&r.Key, &r.Raw) }), nil
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
	query, params, err := db.sb.Delete(db.table).
		Where(hasKeysCondition(namespace, keys...)).
		ToSql()
	if err != nil {
		return collections.RepeatValue(keys, errors.Wrapf(err, "failed to build query"))
	}

	logger.Debug(query, params)
	_, execErr := tx.ExecContext(ctx, query, params...)
	if execErr != nil {
		errs := make(map[driver2.PKey]error)
		for _, key := range keys {
			errs[key] = errors.Wrapf(db.errorWrapper.WrapError(execErr), "could not deleteOp val for key [%s]", key)
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
		if len(val) == 0 {
			logger.DebugfContext(ctx, "set key [%s:%s] to nil value, will be deleted instead", ns, pkey)
			deleted = append(deleted, pkey)
		} else {
			logger.DebugfContext(ctx, "set state [%s,%s]", ns, pkey)
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
	insert := db.sb.Insert(db.table).Columns("ns", "pkey", "val")
	for pkey, val := range vals {
		insert = insert.Values(ns, []byte(pkey), val)
	}
	query, params, err := insert.
		Suffix("ON CONFLICT (ns, pkey) DO UPDATE SET val = EXCLUDED.val").
		ToSql()
	if err != nil {
		return collections.RepeatValue(collections.Keys(vals), errors.Wrapf(err, "failed to build query"))
	}

	logger.Debug(query, params)
	if _, execErr := tx.ExecContext(ctx, query, params...); execErr != nil {
		return collections.RepeatValue(collections.Keys(vals), errors.Wrapf(db.errorWrapper.WrapError(execErr), "could not upsert"))
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
