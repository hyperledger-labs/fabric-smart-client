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
	"github.com/pkg/errors"
)

type VersionedPersistence struct {
	p *common.VersionedPersistence

	writeDB *sql.DB
}

func NewVersioned(opts common.Opts, table string) (*VersionedPersistence, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newVersioned(readWriteDB, table), nil
}

func (db *VersionedPersistence) SetState(namespace driver2.Namespace, key driver2.PKey, value driver.VersionedValue) error {
	return db.p.SetState(namespace, key, value)
}

func (db *VersionedPersistence) SetStates(namespace driver2.Namespace, kvs map[driver2.PKey]driver.VersionedValue) map[driver2.PKey]error {
	return db.p.SetStates(namespace, kvs)
}

func (db *VersionedPersistence) GetState(namespace driver2.Namespace, key driver2.PKey) (driver.VersionedValue, error) {
	return db.p.GetState(namespace, key)
}

func (db *VersionedPersistence) DeleteState(namespace driver2.Namespace, key driver2.PKey) error {
	return db.p.DeleteState(namespace, key)
}

func (db *VersionedPersistence) DeleteStates(namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	return db.p.DeleteStates(namespace, keys...)
}

func (db *VersionedPersistence) GetStateRangeScanIterator(namespace driver2.Namespace, startKey, endKey driver2.PKey) (collections.Iterator[*driver.VersionedRead], error) {
	return db.p.GetStateRangeScanIterator(namespace, startKey, endKey)
}

func (db *VersionedPersistence) GetStateSetIterator(ns driver2.Namespace, keys ...driver2.PKey) (collections.Iterator[*driver.VersionedRead], error) {
	return db.p.GetStateSetIterator(ns, keys...)
}

func (db *VersionedPersistence) Close() error {
	return db.p.Close()
}

func (db *VersionedPersistence) BeginUpdate() error {
	return db.p.BeginUpdate()
}

func (db *VersionedPersistence) Commit() error {
	return db.p.Commit()
}

func (db *VersionedPersistence) Discard() error {
	return db.p.Discard()
}

func (db *VersionedPersistence) GetStateMetadata(namespace driver2.Namespace, key driver2.PKey) (driver2.Metadata, driver2.RawVersion, error) {
	return db.p.GetStateMetadata(namespace, key)
}

func (db *VersionedPersistence) SetStateMetadata(namespace driver2.Namespace, key driver2.PKey, metadata driver2.Metadata, version driver2.RawVersion) error {
	return db.p.SetStateMetadata(namespace, key, metadata, version)
}

func (db *VersionedPersistence) SetStateMetadatas(ns driver2.Namespace, kvs map[driver2.PKey]driver2.VersionedMetadataValue) map[driver2.PKey]error {
	return db.p.SetStateMetadatas(ns, kvs)
}

func (db *VersionedPersistence) CreateSchema() error {
	return db.p.CreateSchema()
}

func (db *VersionedPersistence) SetStateWithTx(tx *sql.Tx, ns driver2.Namespace, pkey driver2.PKey, value driver.VersionedValue) error {
	return db.p.SetStateWithTx(tx, ns, pkey, value)
}

func (db *VersionedPersistence) DeleteStateWithTx(tx *sql.Tx, namespace driver2.Namespace, key driver2.PKey) error {
	return db.p.DeleteStateWithTx(tx, namespace, key)
}

func (db *VersionedPersistence) NewWriteTransaction() (driver.WriteTransaction, error) {
	txn, err := db.writeDB.Begin()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to begin transaction")
	}

	return common.NewWriteTransaction(txn, db), nil
}

type versionedPersistenceNotifier struct {
	driver.VersionedPersistence
	driver.Notifier
}

func (db *versionedPersistenceNotifier) CreateSchema() error {
	if err := db.VersionedPersistence.(*VersionedPersistence).CreateSchema(); err != nil {
		return err
	}
	return db.Notifier.(*Notifier).CreateSchema()
}

func NewVersionedNotifier(opts common.Opts, table string) (*versionedPersistenceNotifier, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return &versionedPersistenceNotifier{
		VersionedPersistence: newVersioned(readWriteDB, table),
		Notifier:             NewNotifier(readWriteDB, table, opts.DataSource, AllOperations, "ns", "pkey"),
	}, nil
}

func newVersioned(readWriteDB *sql.DB, table string) *VersionedPersistence {
	em := &errorMapper{}
	ci := NewInterpreter()
	base := &BasePersistence[driver.VersionedValue, driver.VersionedRead]{
		BasePersistence: common.NewBasePersistence[driver.VersionedValue, driver.VersionedRead](readWriteDB, readWriteDB, table, common.NewVersionedReadScanner(), common.NewVersionedValueScanner(), em, ci, readWriteDB.Begin),
		table:           table,
		ci:              ci,
		errorWrapper:    em,
	}
	return &VersionedPersistence{
		p:       common.NewVersionedPersistence(base, table, em, readWriteDB, readWriteDB),
		writeDB: readWriteDB,
	}
}
