/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

const sqlitePragmas = `
	PRAGMA journal_mode = WAL;
	PRAGMA busy_timeout = 5000;
	PRAGMA synchronous = NORMAL;
	PRAGMA cache_size = 1000000000;
	PRAGMA temp_store = memory;`

const driverName = "sqlite"

var logger = flogging.MustGetLogger("postgres-db")

func NewUnversioned(opts common.Opts, table string) (*common.Unversioned, error) {
	readDB, writeDB, err := openDB(opts.DataSource, opts.MaxOpenConns, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return common.NewUnversioned(readDB, writeDB, table, &errorMapper{}), nil
}

func NewPersistence(opts common.Opts, table string) (*common.Persistence, error) {
	readDB, writeDB, err := openDB(opts.DataSource, opts.MaxOpenConns, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return common.NewPersistence(readDB, writeDB, table, &errorMapper{}), nil
}

func openDB(dataSourceName string, maxOpenConns int, skipPragmas bool) (readDB *sql.DB, writeDB *sql.DB, err error) {
	readDB, err = sql.Open(driverName, dataSourceName)
	if err != nil {
		logger.Error(err)
		if strings.Contains(err.Error(), "out of memory (14)") {
			return nil, nil, fmt.Errorf("can't open %s database, does the folder exist?: %w", driverName, err)
		}
		return nil, nil, fmt.Errorf("can't open %s database: %w", driverName, err)
	}
	readDB.SetMaxOpenConns(maxOpenConns)
	if err = readDB.Ping(); err != nil {
		return nil, nil, err
	}
	logger.Infof("connected to [%s] for reads, max open connections: %d", driverName, maxOpenConns)

	// sqlite can handle concurrent reads in WAL mode if the writes are throttled in 1 connection
	writeDB, err = sql.Open(driverName, dataSourceName)
	if err != nil {
		logger.Error(err)
		return nil, nil, fmt.Errorf("can't open sql database: %w", err)
	}
	writeDB.SetMaxOpenConns(1)
	if err = writeDB.Ping(); err != nil {
		return nil, nil, err
	}
	if skipPragmas {
		if !strings.Contains(dataSourceName, "WAL") {
			logger.Warn("skipping default pragmas. Set at least ?_pragma=journal_mode(WAL) or similar in the dataSource to prevent SQLITE_BUSY errors")
		}
	} else {
		logger.Debug(sqlitePragmas)
		if _, err = readDB.Exec(sqlitePragmas); err != nil {
			return nil, nil, fmt.Errorf("error setting pragmas: %w", err)
		}
		if _, err = writeDB.Exec(sqlitePragmas); err != nil {
			return nil, nil, fmt.Errorf("error setting pragmas: %w", err)
		}
	}
	logger.Infof("connected to [%s] for writes, max open connections: 1", driverName)

	return readDB, writeDB, nil
}
