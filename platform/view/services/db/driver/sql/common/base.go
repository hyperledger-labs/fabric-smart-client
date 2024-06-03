/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
)

var logger = flogging.MustGetLogger("db.driver.sql")

type scannable interface {
	Scan(dest ...any) error
}

type readScanner[V any] interface {
	// Columns returns a slice of the columns to be read into the V struct
	Columns() []string
	// ReadValue reads the columns in the order given above into a V struct
	ReadValue(scannable) (V, error)
}

type valueScanner[V any] interface {
	readScanner[V]
	// WriteValue writes the values of the V struct in the order given by the Columns method
	WriteValue(V) []any
}

type basePersistence[V any, R any] struct {
	writeDB    *sql.DB
	readDB     *sql.DB
	txn        *sql.Tx
	txnLock    sync.Mutex
	debugStack []byte
	table      string

	readScanner  readScanner[R]
	valueScanner valueScanner[V]
	errorWrapper driver.SQLErrorWrapper
}

func (db *basePersistence[V, R]) GetStateRangeScanIterator(ns core.Namespace, startKey, endKey string) (collections.Iterator[*R], error) {
	where, args := rangeWhere(ns, startKey, endKey)
	query := fmt.Sprintf("SELECT %s FROM %s WHERE ns = $1 %s ORDER BY pkey;", strings.Join(db.readScanner.Columns(), ", "), db.table, where)
	logger.Debug(query, ns, startKey, endKey)

	rows, err := db.readDB.Query(query, args...)
	if err != nil {
		return nil, errors2.Wrapf(err, "query error: %s", query)
	}

	return &readIterator[R]{txs: rows, scanner: db.readScanner}, nil
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

func (db *basePersistence[V, R]) GetState(namespace core.Namespace, key string) (V, error) {
	query := fmt.Sprintf("SELECT %s FROM %s WHERE ns = $1 AND pkey = $2", strings.Join(db.valueScanner.Columns(), ", "), db.table)
	logger.Debug(query, namespace, key)

	row := db.readDB.QueryRow(query, namespace, key)
	if value, err := db.valueScanner.ReadValue(row); err == nil {
		return value, nil
	} else if err == sql.ErrNoRows {
		logger.Debugf("not found: [%s:%s]", namespace, key)
		return value, nil
	} else {
		return value, errors2.Wrapf(err, "error querying db: %s", query)
	}
}

func (db *basePersistence[V, R]) SetState(ns core.Namespace, pkey string, value V) error {
	return db.setState(db.txn, ns, pkey, value)
}

func (db *basePersistence[V, R]) setState(tx *sql.Tx, ns core.Namespace, pkey string, value V) error {
	keys := db.valueScanner.Columns()
	values := db.valueScanner.WriteValue(value)
	// Get rawVal
	valIndex := slices.Index(keys, "val")
	val := values[valIndex].([]byte)

	if len(val) == 0 {
		logger.Warnf("set key [%s:%s] to nil value, will be deleted instead", ns, pkey)
		return db.DeleteState(ns, pkey)
	}
	if tx == nil {
		panic("programming error, writing without ongoing update")
	}
	logger.Debugf("set state [%s,%s]", ns, pkey)

	// Overwrite rawVal
	val = append([]byte(nil), val...)
	values[valIndex] = val

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

		query := fmt.Sprintf("UPDATE %s SET %s WHERE ns = $%d AND pkey = $%d", db.table, strings.Join(sets, ", "), len(keys)+1, len(keys)+2)
		logger.Debug(query, ns, pkey, values)

		_, err := tx.Exec(query, append(values, ns, pkey)...)
		if err != nil {
			return errors2.Wrapf(db.errorWrapper.WrapError(err), "could not set val for key [%s]", pkey)
		}
	} else {
		keys = append(keys, "ns", "pkey")
		values = append(values, ns, pkey)
		query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", db.table, strings.Join(keys, ", "), generateParamSet(1, len(keys)))
		logger.Debug(query, ns, pkey, values)

		_, err := tx.Exec(query, values...)
		if err != nil {
			return errors2.Wrapf(db.errorWrapper.WrapError(err), "could not insert [%s]", pkey)
		}
	}

	return nil
}

func (db *basePersistence[V, R]) GetStateSetIterator(ns core.Namespace, keys ...string) (collections.Iterator[*R], error) {
	if len(keys) == 0 {
		return collections.NewEmptyIterator[*R](), nil
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE ns = $1 AND pkey IN %s", strings.Join(db.readScanner.Columns(), ", "), db.table, generateParamSet(2, len(keys)))
	logger.Debug(query, ns, keys)

	rows, err := db.readDB.Query(query, append([]any{ns}, castAny(keys)...)...)
	if err != nil {
		return nil, errors2.Wrapf(err, "query error: %s", query)
	}

	return &readIterator[R]{txs: rows, scanner: db.readScanner}, nil
}

func generateParamSet(offset, count int) string {
	params := make([]string, count)
	for i := 0; i < count; i++ {
		params[i] = fmt.Sprintf("$%d", i+offset)
	}
	return fmt.Sprintf("(%s)", strings.Join(params, ", "))
}

func castAny[A any](as []A) []any {
	if as == nil {
		return nil
	}
	bs := make([]any, len(as))
	for i, a := range as {
		bs[i] = a
	}
	return bs
}

func (db *basePersistence[V, R]) Close() error {
	logger.Info("closing database")

	// TODO: what to do with db.txn if it's not nil?

	err := db.writeDB.Close()
	if err != nil {
		return errors2.Wrapf(err, "could not close DB")
	}

	return nil
}

func (db *basePersistence[V, R]) BeginUpdate() error {
	logger.Debugf("begin db transaction [%s]", db.table)
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.txn != nil {
		logger.Errorf("previous commit in progress, locked by [%s]", db.debugStack)
		return errors.New("previous commit in progress")
	}

	tx, err := db.writeDB.Begin()
	if err != nil {
		return errors2.Wrapf(err, "error starting db transaction")
	}
	db.txn = tx
	db.debugStack = debug.Stack()

	return nil
}

func (db *basePersistence[V, R]) Commit() error {
	logger.Debugf("commit db transaction [%s]", db.table)
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.txn == nil {
		return errors.New("no commit in progress")
	}

	err := db.txn.Commit()
	db.txn = nil
	if err != nil {
		return errors2.Wrapf(err, "could not commit transaction")
	}

	return nil
}

func (db *basePersistence[V, R]) Discard() error {
	logger.Debug(fmt.Sprintf("rollback db transaction on table %s", db.table))
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.txn == nil {
		return errors.New("no commit in progress")
	}
	err := db.txn.Rollback()
	db.txn = nil
	if err != nil {
		logger.Infof("error rolling back (ignoring): %s", err.Error())
		return nil
	}

	return nil
}

func (db *basePersistence[V, R]) exists(tx *sql.Tx, ns, key string) (bool, error) {
	var pkey string
	query := fmt.Sprintf("SELECT pkey FROM %s WHERE ns = $1 AND pkey = $2", db.table)
	logger.Debug(query, ns, key)
	err := tx.QueryRow(query, ns, key).Scan(&pkey)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, errors2.Wrapf(err, "cannot check if key exists: %s", key)
	}
	return true, nil
}

func (db *basePersistence[V, R]) DeleteState(ns, key string) error {
	if db.txn == nil {
		panic("programming error, writing without ongoing update")
	}
	if ns == "" || key == "" {
		return errors.New("ns or key is empty")
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE ns = $1 AND pkey = $2", db.table)
	logger.Debug(query, ns, key)
	_, err := db.txn.Exec(query, ns, key)
	if err != nil {
		return errors2.Wrapf(db.errorWrapper.WrapError(err), "could not delete val for key [%s]", key)
	}

	return nil
}

func (db *basePersistence[V, R]) createSchema(query string) error {
	logger.Debug(query)
	if _, err := db.writeDB.Exec(query); err != nil {
		return errors2.Wrapf(err, "can't create table")
	}
	return nil
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

type Opts struct {
	Driver          string
	DataSource      string
	TablePrefix     string
	SkipCreateTable bool
	SkipPragmas     bool
	MaxOpenConns    int
}
