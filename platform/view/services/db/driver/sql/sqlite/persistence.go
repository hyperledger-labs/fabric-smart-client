/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

const (
	maxIdleConnsWrite               = 1
	maxIdleTimeWrite  time.Duration = 0
)

const sqlitePragmas = `
	PRAGMA journal_mode = WAL;
	PRAGMA busy_timeout = 5000;
	PRAGMA synchronous = NORMAL;
	PRAGMA cache_size = 1000000000;
	PRAGMA temp_store = memory;
	PRAGMA foreign_keys = ON;`

const driverName = "sqlite"

var logger = logging.MustGetLogger("view-sdk.db.sqlite")

func OpenRWDBs(dataSourceName string, maxOpenConns int, maxIdleConns *int, maxIdleTime *time.Duration, skipPragmas bool) (*sql.DB, *sql.DB, error) {
	logger.Infof("Opening read db [%v]", dataSourceName)
	readDB, err := OpenDB(dataSourceName, maxOpenConns, maxIdleConns, maxIdleTime, skipPragmas)
	if err != nil {
		return nil, nil, fmt.Errorf("can't open read %s database: %w", driverName, err)
	}
	logger.Infof("Opening write db [%v]", dataSourceName)
	writeDB, err := OpenDB(dataSourceName, 1, common.CopyPtr(maxIdleConnsWrite), common.CopyPtr(maxIdleTimeWrite), skipPragmas)
	if err != nil {
		return nil, nil, fmt.Errorf("can't open write %s database: %w", driverName, err)
	}
	return readDB, writeDB, nil
}

func OpenDB(dataSourceName string, maxOpenConns int, maxIdleConns *int, maxIdleTime *time.Duration, skipPragmas bool) (*sql.DB, error) {
	// Create directories if they do not exist to avoid error "out of memory (14)", see below
	path := getDir(dataSourceName)
	if err := os.MkdirAll(path, 0777); err != nil {
		logger.Warnf("failed creating dir [%s]: %s", path, err)
	}

	if maxIdleConns == nil {
		maxIdleConns = common.CopyPtr(common.DefaultMaxIdleConns)
	}
	if maxIdleTime == nil {
		maxIdleTime = common.CopyPtr(common.DefaultMaxIdleTime)
	}

	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("can't open %s database: %w", driverName, err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(*maxIdleConns)
	db.SetConnMaxIdleTime(*maxIdleTime)

	if err = db.Ping(); err != nil && strings.Contains(err.Error(), "out of memory (14)") {
		return nil, fmt.Errorf("can't open %s database, does the folder exist?", driverName)
	} else if err != nil {
		return nil, err
	}
	logger.Infof("connected to [%s], max open connections: %d", driverName, maxOpenConns)

	// sqlite can handle concurrent reads in WAL mode if the writes are throttled in 1 connection
	if skipPragmas {
		if !strings.Contains(dataSourceName, "WAL") {
			logger.Warn("skipping default pragmas. Set at least ?_pragma=journal_mode(WAL) or similar in the dataSource to prevent SQLITE_BUSY errors")
		}
		return db, nil
	}
	logger.Debug(sqlitePragmas)
	if _, err = db.Exec(sqlitePragmas); err != nil {
		return nil, fmt.Errorf("error setting pragmas: %w", err)
	}

	return db, nil
}

func getDir(dataSourceName string) string {
	if strings.HasPrefix(dataSourceName, "file:") {
		u, err := url.Parse(dataSourceName)
		if err != nil {
			return ""
		}
		return filepath.Dir(u.Path)
	}
	return filepath.Dir(dataSourceName)
}
