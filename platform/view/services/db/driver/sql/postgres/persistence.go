/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("postgres-db")
var notifyOperations = []driver.Operation{driver.Insert, driver.Update, driver.Delete}

const driverName = "postgres"

func NewUnversioned(opts common.Opts, table string) (*common.Unversioned, error) {
	readWriteDB, err := openDB(opts.DataSource, opts.MaxOpenConns)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return common.NewUnversioned(readWriteDB, readWriteDB, table, &errorMapper{}), nil
}

func NewUnversionedNotifier(opts common.Opts, table string) (*unversionedPersistenceNotifier, error) {
	readWriteDB, err := openDB(opts.DataSource, opts.MaxOpenConns)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return &unversionedPersistenceNotifier{
		Unversioned: common.NewUnversioned(readWriteDB, readWriteDB, table, &errorMapper{}),
		notifier:    newNotifier(readWriteDB, table, opts.DataSource, notifyOperations, "ns", "pkey"),
	}, nil
}

func NewPersistence(opts common.Opts, table string) (*common.Persistence, error) {
	readWriteDB, err := openDB(opts.DataSource, opts.MaxOpenConns)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return common.NewPersistence(readWriteDB, readWriteDB, table, &errorMapper{}), nil
}

func NewPersistenceNotifier(opts common.Opts, table string) (*versionedPersistenceNotifier, error) {
	readWriteDB, err := openDB(opts.DataSource, opts.MaxOpenConns)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return &versionedPersistenceNotifier{
		Persistence: common.NewPersistence(readWriteDB, readWriteDB, table, &errorMapper{}),
		notifier:    newNotifier(readWriteDB, table, opts.DataSource, notifyOperations, "ns", "pkey"),
	}, nil
}

func openDB(dataSourceName string, maxOpenConns int) (*sql.DB, error) {
	readDB, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		logger.Error(err)
		if strings.Contains(err.Error(), "out of memory (14)") {
			return nil, fmt.Errorf("can't open %s database, does the folder exist?: %w", driverName, err)
		}
		return nil, fmt.Errorf("can't open %s database: %w", driverName, err)
	}
	readDB.SetMaxOpenConns(maxOpenConns)
	if err = readDB.Ping(); err != nil {
		return nil, err
	}
	logger.Infof("connected to [%s] for reads, max open connections: %d", driverName, maxOpenConns)

	logger.Info("using same db for writes")

	return readDB, nil
}
