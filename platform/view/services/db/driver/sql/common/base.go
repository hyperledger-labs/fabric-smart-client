/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"fmt"
	"strings"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
)

var logger = flogging.MustGetLogger("view-sdk.db.driver.sql")

type scannable interface {
	Scan(dest ...any) error
}

type readScanner[V any] interface {
	// Columns returns a slice of the columns to be read into the V struct
	Columns() []string
	// ReadValue reads the columns in the order given above into a V struct
	ReadValue(scannable) (V, error)
}

type ValueScanner[V any] interface {
	readScanner[V]
	// WriteValue writes the values of the V struct in the order given by the Columns method
	WriteValue(V) []any
}

type BasePersistence[V any, R any] struct {
	*common.BaseDB[*sql.Tx]
	writeDB *sql.DB
	readDB  *sql.DB
	table   string

	readScanner  readScanner[R]
	ValueScanner ValueScanner[V]
	errorWrapper driver.SQLErrorWrapper
	ci           Interpreter
}

func NewBasePersistence[V any, R any](writeDB *sql.DB, readDB *sql.DB, table string, readScanner readScanner[R], valueScanner ValueScanner[V], errorWrapper driver.SQLErrorWrapper, ci Interpreter, newTransaction func() (*sql.Tx, error)) *BasePersistence[V, R] {
	return &BasePersistence[V, R]{
		BaseDB:       common.NewBaseDB[*sql.Tx](func() (*sql.Tx, error) { return newTransaction() }),
		readDB:       readDB,
		writeDB:      writeDB,
		table:        table,
		readScanner:  readScanner,
		ValueScanner: valueScanner,
		errorWrapper: errorWrapper,
		ci:           ci,
	}
}

func (db *BasePersistence[V, R]) GetStateRangeScanIterator(ns driver2.Namespace, startKey, endKey string) (collections.Iterator[*R], error) {
	where, args := Where(db.ci.And(
		db.ci.Cmp("ns", "=", ns),
		db.ci.BetweenStrings("pkey", startKey, endKey),
	))
	query := fmt.Sprintf("SELECT %s FROM %s %s ORDER BY pkey;", strings.Join(db.readScanner.Columns(), ", "), db.table, where)
	logger.Debug(query, ns, startKey, endKey)

	rows, err := db.readDB.Query(query, args...)
	if err != nil {
		return nil, errors2.Wrapf(err, "query error: %s", query)
	}

	return &readIterator[R]{txs: rows, scanner: db.readScanner}, nil
}

func (db *BasePersistence[V, R]) GetState(namespace driver2.Namespace, key string) (V, error) {
	where, args := Where(db.hasKey(namespace, key))
	query := fmt.Sprintf("SELECT %s FROM %s %s", strings.Join(db.ValueScanner.Columns(), ", "), db.table, where)
	logger.Debug(query, args)

	row := db.readDB.QueryRow(query, args...)
	if value, err := db.ValueScanner.ReadValue(row); err == nil {
		return value, nil
	} else if err == sql.ErrNoRows {
		logger.Debugf("not found: [%s:%s]", namespace, key)
		return value, nil
	} else {
		return value, errors2.Wrapf(err, "error querying db: %s", query)
	}
}

func (db *BasePersistence[V, R]) SetState(ns driver2.Namespace, pkey driver2.PKey, value V) error {
	return db.SetStateWithTx(db.Txn, ns, pkey, value)
}

func (db *BasePersistence[V, R]) SetStates(ns driver2.Namespace, kvs map[driver2.PKey]V) map[driver2.PKey]error {
	errs := make(map[driver2.PKey]error)
	for pkey, value := range kvs {
		if err := db.SetState(ns, pkey, value); err != nil {
			errs[pkey] = err
		}
	}
	return errs
}

func (db *BasePersistence[V, R]) GetStateSetIterator(ns driver2.Namespace, keys ...driver2.PKey) (collections.Iterator[*R], error) {
	if len(keys) == 0 {
		return collections.NewEmptyIterator[*R](), nil
	}
	where, args := Where(db.ci.And(
		db.ci.Cmp("ns", "=", ns),
		db.ci.InStrings("pkey", keys),
	))
	query := fmt.Sprintf("SELECT %s FROM %s %s", strings.Join(db.readScanner.Columns(), ", "), db.table, where)
	logger.Debug(query[:30] + "...")

	rows, err := db.readDB.Query(query, args...)
	if err != nil {
		return nil, errors2.Wrapf(err, "query error: %s", query)
	}

	return &readIterator[R]{txs: rows, scanner: db.readScanner}, nil
}

func (db *BasePersistence[V, R]) Close() error {
	logger.Info("closing database")

	// TODO: what to do with db.Txn if it's not nil?

	err := db.writeDB.Close()
	if err != nil {
		return errors2.Wrapf(err, "could not close DB")
	}

	return nil
}

func (db *BasePersistence[V, R]) DeleteState(ns driver2.Namespace, key driver2.PKey) error {
	return db.DeleteStateWithTx(db.Txn, ns, key)
}

func (db *BasePersistence[V, R]) DeleteStateWithTx(tx *sql.Tx, ns driver2.Namespace, key driver2.PKey) error {
	if errs := db.DeleteStatesWithTx(tx, ns, key); errs != nil {
		return errs[key]
	}
	return nil
}

func (db *BasePersistence[V, R]) DeleteStates(namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	return db.DeleteStatesWithTx(db.Txn, namespace, keys...)
}

func (db *BasePersistence[V, R]) DeleteStatesWithTx(tx *sql.Tx, namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
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

func (db *BasePersistence[V, R]) hasKeys(ns driver2.Namespace, pkeys []driver2.PKey) Condition {
	return db.ci.And(
		db.ci.Cmp("ns", "=", ns),
		db.ci.InStrings("pkey", pkeys),
	)
}

func (db *BasePersistence[V, R]) SetStateWithTx(tx *sql.Tx, ns driver2.Namespace, pkey string, value V) error {
	if tx == nil {
		panic("programming error, writing without ongoing update")
	}

	keys := db.ValueScanner.Columns()
	values := db.ValueScanner.WriteValue(value)
	// Get rawVal
	valIndex := slices.Index(keys, "val")
	val := values[valIndex].([]byte)
	if len(val) == 0 {
		logger.Debugf("set key [%s:%s] to nil value, will be deleted instead", ns, pkey)
		return db.DeleteState(ns, pkey)
	}

	logger.Debugf("set state [%s,%s]", ns, pkey)

	// Overwrite rawVal
	val = append([]byte(nil), val...)
	values[valIndex] = val

	return db.UpsertStateWithTx(tx, ns, pkey, keys, values)
}

func (db *BasePersistence[V, R]) UpsertStates(ns driver2.Namespace, valueKeys []string, vals map[driver2.PKey][]any) map[driver2.PKey]error {
	errs := make(map[driver2.PKey]error)
	for pkey, val := range vals {
		if err := db.UpsertStateWithTx(db.Txn, ns, pkey, valueKeys, val); err != nil {
			errs[pkey] = err
		}
	}
	return errs
}

func (db *BasePersistence[V, R]) UpsertStateWithTx(tx *sql.Tx, ns driver2.Namespace, pkey driver2.PKey, keys []string, values []any) error {
	// Portable upsert
	exists, err := db.exists(tx, ns, pkey)
	if err != nil {
		return err
	}

	if exists {
		sets := make([]string, len(keys))
		for i, key := range keys {
			sets[i] = fmt.Sprintf("%s = $%d", key, i+1)
		}

		cond := db.hasKey(ns, pkey)
		offset := len(keys) + 1
		where, args := cond.ToString(&offset), cond.Params()
		query := fmt.Sprintf("UPDATE %s SET %s WHERE %s", db.table, strings.Join(sets, ", "), where)
		logger.Debug(query, args, len(values))

		_, err := tx.Exec(query, append(values, args...)...)
		if err != nil {
			return errors2.Wrapf(db.errorWrapper.WrapError(err), "could not set val for key [%s]", pkey)
		}
	} else {
		keys = append(keys, "ns", "pkey")
		values = append(values, ns, pkey)
		query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", db.table, strings.Join(keys, ", "), generateParamSet(1, len(keys)))
		logger.Debug(query, ns, pkey, len(values))

		_, err := tx.Exec(query, values...)
		if err != nil {
			return errors2.Wrapf(db.errorWrapper.WrapError(err), "could not insert [%s]", pkey)
		}
	}
	logger.Debugf("set state [%s,%s], done", ns, pkey)

	return nil
}

func (db *BasePersistence[V, R]) Exec(query string, args ...any) (sql.Result, error) {
	return db.Txn.Exec(query, args...)
}

func (db *BasePersistence[V, R]) Exists(ns driver2.Namespace, key driver2.PKey) (bool, error) {
	if db.Txn == nil {
		panic("programming error, writing without ongoing update")
	}
	return db.exists(db.Txn, ns, key)
}

func (db *BasePersistence[V, R]) exists(tx *sql.Tx, ns driver2.Namespace, key driver2.PKey) (bool, error) {
	var pkey string
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

func (db *BasePersistence[V, R]) hasKey(ns driver2.Namespace, pkey string) Condition {
	return db.ci.And(
		db.ci.Cmp("ns", "=", ns),
		db.ci.Cmp("pkey", "=", pkey),
	)
}

func (db *BasePersistence[V, R]) InitSchema(query string) error {
	return InitSchema(db.writeDB, query)
}

func (db *BasePersistence[V, R]) TableName() string {
	return db.table
}

type readIterator[V any] struct {
	txs     *sql.Rows
	scanner readScanner[V]
}

func (t *readIterator[V]) Close() {
	t.txs.Close()
}

func (t *readIterator[V]) Next() (*V, error) {
	if !t.txs.Next() {
		return nil, nil
	}

	r, err := t.scanner.ReadValue(t.txs)

	return &r, err
}

type SQLDriverType string

type Opts struct {
	Driver          SQLDriverType
	DataSource      string
	TablePrefix     string
	SkipCreateTable bool
	SkipPragmas     bool
	MaxOpenConns    int
}

func generateParamSet(offset, count int) string {
	params := make([]string, count)
	for i := 0; i < count; i++ {
		params[i] = fmt.Sprintf("$%d", i+offset)
	}
	return fmt.Sprintf("(%s)", strings.Join(params, ", "))
}
