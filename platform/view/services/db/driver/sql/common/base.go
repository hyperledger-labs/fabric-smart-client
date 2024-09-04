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

type valueScanner[V any] interface {
	readScanner[V]
	// WriteValue writes the values of the V struct in the order given by the Columns method
	WriteValue(V) []any
}

type basePersistence[V any, R any] struct {
	*common.BaseDB[*sql.Tx]
	writeDB *sql.DB
	readDB  *sql.DB
	table   string

	readScanner  readScanner[R]
	valueScanner valueScanner[V]
	errorWrapper driver.SQLErrorWrapper
	ci           Interpreter
}

func (db *basePersistence[V, R]) GetStateRangeScanIterator(ns driver2.Namespace, startKey, endKey string) (collections.Iterator[*R], error) {
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

func (db *basePersistence[V, R]) GetState(namespace driver2.Namespace, key string) (V, error) {
	where, args := Where(db.hasKey(namespace, key))
	query := fmt.Sprintf("SELECT %s FROM %s %s", strings.Join(db.valueScanner.Columns(), ", "), db.table, where)
	logger.Debug(query, args)

	row := db.readDB.QueryRow(query, args...)
	if value, err := db.valueScanner.ReadValue(row); err == nil {
		return value, nil
	} else if err == sql.ErrNoRows {
		logger.Debugf("not found: [%s:%s]", namespace, key)
		return value, nil
	} else {
		return value, errors2.Wrapf(err, "error querying db: %s", query)
	}
}

func (db *basePersistence[V, R]) SetState(ns driver2.Namespace, pkey string, value V) error {
	return db.setState(db.Txn, ns, pkey, value)
}

func (db *basePersistence[V, R]) GetStateSetIterator(ns driver2.Namespace, keys ...string) (collections.Iterator[*R], error) {
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

func (db *basePersistence[V, R]) Close() error {
	logger.Info("closing database")

	// TODO: what to do with db.Txn if it's not nil?

	err := db.writeDB.Close()
	if err != nil {
		return errors2.Wrapf(err, "could not close DB")
	}

	return nil
}

func (db *basePersistence[V, R]) DeleteState(ns driver2.Namespace, key string) error {
	if db.Txn == nil {
		panic("programming error, writing without ongoing update")
	}
	if ns == "" || key == "" {
		return errors.New("ns or key is empty")
	}
	where, args := Where(db.hasKey(ns, key))
	query := fmt.Sprintf("DELETE FROM %s %s", db.table, where)
	logger.Debug(query, args)
	_, err := db.Txn.Exec(query, args...)
	if err != nil {
		return errors2.Wrapf(db.errorWrapper.WrapError(err), "could not delete val for key [%s]", key)
	}

	return nil
}

func (db *basePersistence[V, R]) setState(tx *sql.Tx, ns driver2.Namespace, pkey string, value V) error {
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

		cond := db.hasKey(ns, pkey)
		offset := len(keys) + 1
		where, args := cond.ToString(&offset), cond.Params()
		query := fmt.Sprintf("UPDATE %s SET %s WHERE %s", db.table, strings.Join(sets, ", "), where)
		logger.Infof(query, args, len(values))

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

func (db *basePersistence[V, R]) exists(tx *sql.Tx, ns, key string) (bool, error) {
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

func (db *basePersistence[V, R]) hasKey(ns driver2.Namespace, pkey string) Condition {
	return db.ci.And(
		db.ci.Cmp("ns", "=", ns),
		db.ci.Cmp("pkey", "=", pkey),
	)
}

func (db *basePersistence[V, R]) createSchema(query string) error {
	return InitSchema(db.writeDB, query)
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
