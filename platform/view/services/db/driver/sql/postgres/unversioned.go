/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type UnversionedPersistence struct {
	*common.UnversionedPersistence
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
		Notifier:               NewNotifier(readWriteDB, table, opts.DataSource, AllOperations, "ns", "pkey"),
	}, nil
}

func newUnversioned(readWriteDB *sql.DB, table string) *UnversionedPersistence {
	base := &BasePersistence[driver.UnversionedValue, driver.UnversionedRead]{
		BasePersistence: common.NewBasePersistence[driver.UnversionedValue, driver.UnversionedRead](readWriteDB, readWriteDB, table, common.NewUnversionedReadScanner(), common.NewUnversionedValueScanner(), &errorMapper{}, NewInterpreter(), readWriteDB.Begin),
	}
	return &UnversionedPersistence{
		UnversionedPersistence: common.NewUnversionedPersistence(base, readWriteDB, table),
	}
}
