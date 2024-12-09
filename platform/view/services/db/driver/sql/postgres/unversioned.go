/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"fmt"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type UnversionedPersistence struct {
	*common.UnversionedPersistence
}

func (p *UnversionedPersistence) SetState(namespace driver2.Namespace, key driver2.PKey, value driver.UnversionedValue) error {
	return p.UnversionedPersistence.SetState(namespace, key, value)
}

func (p *UnversionedPersistence) SetStates(namespace driver2.Namespace, kvs map[driver2.PKey]driver.UnversionedValue) map[driver2.PKey]error {
	return p.UnversionedPersistence.SetStates(namespace, kvs)
}

func (p *UnversionedPersistence) GetState(namespace driver2.Namespace, key driver2.PKey) (driver.UnversionedValue, error) {
	return p.UnversionedPersistence.GetState(namespace, key)
}

func (p *UnversionedPersistence) DeleteState(namespace driver2.Namespace, key driver2.PKey) error {
	return p.UnversionedPersistence.DeleteState(namespace, key)
}

func (p *UnversionedPersistence) DeleteStates(namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	return p.UnversionedPersistence.DeleteStates(namespace, keys...)
}

func (p *UnversionedPersistence) GetStateRangeScanIterator(namespace driver2.Namespace, startKey, endKey driver2.PKey) (collections.Iterator[*driver.UnversionedRead], error) {
	return decodeUnversionedReadIterator(p.UnversionedPersistence.GetStateRangeScanIterator(namespace, startKey, endKey))
}

func (p *UnversionedPersistence) GetStateSetIterator(ns driver2.Namespace, keys ...driver2.PKey) (collections.Iterator[*driver.UnversionedRead], error) {
	return decodeUnversionedReadIterator(p.UnversionedPersistence.GetStateSetIterator(ns, keys...))
}

func NewUnversioned(opts common.Opts, table string) (*UnversionedPersistence, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newUnversioned(readWriteDB, table), nil
}

type unversionedPersistenceNotifier struct {
	*UnversionedPersistence
	*Notifier
}

func (db *unversionedPersistenceNotifier) CreateSchema() error {
	if err := db.UnversionedPersistence.CreateSchema(); err != nil {
		return err
	}
	return db.Notifier.CreateSchema()
}

func NewUnversionedNotifier(opts common.Opts, table string) (*unversionedPersistenceNotifier, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return &unversionedPersistenceNotifier{
		UnversionedPersistence: newUnversioned(readWriteDB, table),
		Notifier:               NewNotifier(readWriteDB, table, opts.DataSource, AllOperations, primaryKey{"ns", identity}, primaryKey{"pkey", decode}),
	}, nil
}

func newUnversioned(readWriteDB *sql.DB, table string) *UnversionedPersistence {
	ci := NewInterpreter()
	em := &errorMapper{}
	base := &BasePersistence[driver.UnversionedValue, driver.UnversionedRead]{
		BasePersistence: common.NewBasePersistence[driver.UnversionedValue, driver.UnversionedRead](readWriteDB, readWriteDB, table, common.NewUnversionedReadScanner(), common.NewUnversionedValueScanner(), em, ci, readWriteDB.Begin),
		table:           table,
		ci:              ci,
		errorWrapper:    em,
	}
	return &UnversionedPersistence{
		UnversionedPersistence: common.NewUnversionedPersistence(base, readWriteDB, table),
	}
}
