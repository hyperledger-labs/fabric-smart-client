/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/lib/pq"
)

var logger = flogging.MustGetLogger("view-sdk.db.postgres")

const driverName = "postgres"

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
