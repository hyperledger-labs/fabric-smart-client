/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var logger = logging.MustGetLogger("view-sdk.db.postgres")

const driverName = "pgx"

type Opts struct {
	DataSource   string
	MaxOpenConns int
	MaxIdleConns int
	MaxIdleTime  time.Duration
}

func openDB(opts Opts) (*sql.DB, error) {
	return OpenDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime)
}

func OpenDB(dataSourceName string, maxOpenConns int, maxIdleConns int, maxIdleTime time.Duration) (*sql.DB, error) {
	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		logger.Error(err)
		return nil, fmt.Errorf("can't open %s database: %w", driverName, err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxIdleTime(maxIdleTime)

	if err = db.Ping(); err != nil {
		return nil, err
	}
	logger.Debugf("connected to [%s] for reads, max open connections: %d, max idle connections: %d, max idle time: %v", driverName, maxOpenConns, maxIdleConns, maxIdleTime)

	logger.Debug("using same db for writes")

	return db, nil
}
