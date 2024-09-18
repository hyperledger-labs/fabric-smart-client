/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

const sqlitePragmas = `
	PRAGMA journal_mode = WAL;
	PRAGMA busy_timeout = 5000;
	PRAGMA synchronous = NORMAL;
	PRAGMA cache_size = 1000000000;
	PRAGMA temp_store = memory;
	PRAGMA foreign_keys = ON;`

const driverName = "sqlite"

var logger = flogging.MustGetLogger("view-sdk.db.sqlite")

func openDB(dataSourceName string, maxOpenConns int, skipPragmas bool) (*sql.DB, *sql.DB, error) {
	logger.Infof("Opening read db [%v]", dataSourceName)
	readDB, err := OpenDB(dataSourceName, maxOpenConns, skipPragmas)
	if err != nil {
		return nil, nil, fmt.Errorf("can't open read %s database: %w", driverName, err)
	}
	logger.Infof("Opening write db [%v]", dataSourceName)
	writeDB, err := OpenDB(dataSourceName, 1, skipPragmas)
	if err != nil {
		return nil, nil, fmt.Errorf("can't open write %s database: %w", driverName, err)
	}
	return readDB, writeDB, nil
}

func OpenDB(dataSourceName string, maxOpenConns int, skipPragmas bool) (*sql.DB, error) {
	// Create directories if they do not exist to avoid error "out of memory (14)", see below
	if strings.HasPrefix(dataSourceName, "file:") {
		path := strings.TrimLeft(dataSourceName[:strings.IndexRune(dataSourceName, '?')], "file:")
		if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
			logger.Warnf("failed creating dir [%s]: %s", path, err)
		}
	}

	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("can't open %s database: %w", driverName, err)
	}
	db.SetMaxOpenConns(maxOpenConns)
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
