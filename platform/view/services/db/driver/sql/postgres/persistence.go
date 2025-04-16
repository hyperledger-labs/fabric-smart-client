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
	DataSource      string
	MaxOpenConns    int
	MaxIdleConns    int
	MaxIdleTime     time.Duration
	TablePrefix     string
	TableNameParams []string
}

func OpenDB(opts Opts) (*sql.DB, error) {
	db, err := sql.Open(driverName, opts.DataSource)
	if err != nil {
		logger.Error(err)
		return nil, fmt.Errorf("can't open %s database: %w", driverName, err)
	}

	db.SetMaxOpenConns(opts.MaxOpenConns)
	db.SetMaxIdleConns(opts.MaxIdleConns)
	db.SetConnMaxIdleTime(opts.MaxIdleTime)

	if err = db.Ping(); err != nil {
		return nil, err
	}
	logger.Debugf("connected to [%s] for reads, max open connections: %d, max idle connections: %d, max idle time: %v", driverName, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime)

	logger.Debug("using same db for writes")

	return db, nil
}
