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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("view-sdk.db.postgres")
var AllOperations = []driver.Operation{driver.Insert, driver.Update, driver.Delete}

const driverName = "pgx"

func NewUnversioned(opts common.Opts, table string) (*common.UnversionedPersistence, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return common.NewUnversioned(readWriteDB, readWriteDB, table, &errorMapper{}), nil
}

func NewUnversionedNotifier(opts common.Opts, table string) (*unversionedPersistenceNotifier, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return &unversionedPersistenceNotifier{
		UnversionedPersistence: common.NewUnversioned(readWriteDB, readWriteDB, table, &errorMapper{}),
		Notifier:               NewNotifier(readWriteDB, table, opts.DataSource, AllOperations, "ns", "pkey"),
	}, nil
}

func NewPersistence(opts common.Opts, table string) (*common.VersionedPersistence, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return common.NewVersionedPersistence(readWriteDB, readWriteDB, table, &errorMapper{}), nil
}

func NewPersistenceNotifier(opts common.Opts, table string) (*versionedPersistenceNotifier, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return &versionedPersistenceNotifier{
		VersionedPersistence: common.NewVersionedPersistence(readWriteDB, readWriteDB, table, &errorMapper{}),
		Notifier:             NewNotifier(readWriteDB, table, opts.DataSource, AllOperations, "ns", "pkey"),
	}, nil
}

func OpenDB(dataSourceName string, maxOpenConns int) (*sql.DB, error) {
	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		logger.Error(err)
		return nil, fmt.Errorf("can't open %s database: %w", driverName, err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	if err = db.Ping(); err != nil {
		return nil, err
	}
	logger.Infof("connected to [%s] for reads, max open connections: %d", driverName, maxOpenConns)

	logger.Info("using same db for writes")

	return db, nil
}
