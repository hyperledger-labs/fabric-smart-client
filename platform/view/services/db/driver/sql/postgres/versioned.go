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

type VersionedPersistence struct {
	*common.VersionedPersistence
}

func NewVersioned(opts common.Opts, table string) (*VersionedPersistence, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newVersioned(readWriteDB, table), nil
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
	base := &BasePersistence[driver.VersionedValue, driver.VersionedRead]{
		BasePersistence: common.NewBasePersistence[driver.VersionedValue, driver.VersionedRead](readWriteDB, readWriteDB, table, common.NewVersionedReadScanner(), common.NewVersionedValueScanner(), &errorMapper{}, NewInterpreter(), readWriteDB.Begin),
	}
	return &VersionedPersistence{
		VersionedPersistence: common.NewVersionedPersistence(base, table, &errorMapper{}, readWriteDB, readWriteDB),
	}
}
