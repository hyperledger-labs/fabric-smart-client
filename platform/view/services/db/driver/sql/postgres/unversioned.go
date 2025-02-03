/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"fmt"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type UnversionedPersistence struct {
	*common.UnversionedPersistence

	table        string
	ci           common.Interpreter
	errorWrapper driver2.SQLErrorWrapper
}

func (db *UnversionedPersistence) SetState(ns driver.Namespace, pkey driver.PKey, value driver.UnversionedValue) error {
	return db.SetStateWithTx(db.Txn, ns, pkey, value)
}

func (db *UnversionedPersistence) SetStates(ns driver.Namespace, kvs map[driver.PKey]driver.UnversionedValue) map[driver.PKey]error {
	return db.setStatesWithTx(db.Txn, ns, kvs)
}

func (db *UnversionedPersistence) SetStateWithTx(tx *sql.Tx, ns driver.Namespace, pkey driver.PKey, value driver.UnversionedValue) error {
	if errs := db.setStatesWithTx(tx, ns, map[driver.PKey]driver.UnversionedValue{pkey: value}); errs != nil {
		return errs[encode(pkey)]
	}
	return nil
}

func (db *UnversionedPersistence) GetStateRangeScanIterator(ns driver.Namespace, startKey, endKey string) (collections.Iterator[*driver.UnversionedRead], error) {
	return decodeUnversionedReadIterator(db.UnversionedPersistence.GetStateRangeScanIterator(ns, encode(startKey), encode(endKey)))
}

func (db *UnversionedPersistence) GetStateSetIterator(ns driver.Namespace, keys ...driver.PKey) (collections.Iterator[*driver.UnversionedRead], error) {
	return decodeUnversionedReadIterator(db.UnversionedPersistence.GetStateSetIterator(ns, encodeSlice(keys)...))
}

func (db *UnversionedPersistence) setStatesWithTx(tx *sql.Tx, ns driver.Namespace, kvs map[driver.PKey]driver.UnversionedValue) map[driver.PKey]error {
	if tx == nil {
		panic("programming error, writing without ongoing update")
	}

	upserted := make(map[driver.PKey]driver.UnversionedValue, len(kvs))
	deleted := make([]driver.PKey, 0, len(kvs))
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

	errs := make(map[driver.PKey]error)
	if len(deleted) > 0 {
		collections.CopyMap(errs, db.DeleteStatesWithTx(tx, ns, encodeSlice(deleted)...))
	}
	if len(upserted) > 0 {
		collections.CopyMap(errs, db.upsertStatesWithTx(tx, ns, encodeMap(upserted)))
	}
	return errs
}

func (db *UnversionedPersistence) upsertStatesWithTx(tx *sql.Tx, ns driver.Namespace, vals map[driver.PKey]driver.UnversionedValue) map[driver.PKey]error {

	query := fmt.Sprintf("INSERT INTO %s (ns, pkey, val) "+
		"VALUES %s "+
		"ON CONFLICT (ns, pkey) DO UPDATE "+
		"SET val=excluded.val",
		db.table,
		common.CreateParamsMatrix(3, len(vals), 1))

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

func NewUnversionedPersistence(opts common.Opts, table string) (*UnversionedPersistence, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newUnversionedPersistence(readWriteDB, table), nil
}

type unversionedPersistenceNotifier struct {
	*UnversionedPersistence
	*Notifier
}

func (db *unversionedPersistenceNotifier) CreateSchema() error {
	if err := db.UnversionedPersistence.UnversionedPersistence.CreateSchema(); err != nil {
		return err
	}
	return db.Notifier.CreateSchema()
}

func NewUnversionedNotifier(opts common.Opts, table string) (*unversionedPersistenceNotifier, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return &unversionedPersistenceNotifier{
		UnversionedPersistence: newUnversionedPersistence(readWriteDB, table),
		Notifier:               NewNotifier(readWriteDB, table, opts.DataSource, AllOperations, primaryKey{"ns", identity}, primaryKey{"pkey", decode}),
	}, nil
}

func newUnversionedPersistence(readWriteDB *sql.DB, table string) *UnversionedPersistence {
	ci := NewInterpreter()
	errorWrapper := &errorMapper{}
	return &UnversionedPersistence{
		UnversionedPersistence: common.NewUnversionedPersistence(readWriteDB, readWriteDB, table, errorWrapper, ci),
		table:                  table,
		ci:                     ci,
		errorWrapper:           errorWrapper,
	}
}
