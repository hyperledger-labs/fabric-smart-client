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
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/opentelemetry-go-extra/otelsql"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

var logger = logging.MustGetLogger()

const driverName = "pgx"

type DbProvider interface {
	Get(Opts) (*common.RWDB, error)
}

func NewDbProvider() DbProvider { return lazy.NewProviderWithKeyMapper(key, Open) }

func key(o Opts) string { return o.DataSource }

func Open(opts Opts) (*common.RWDB, error) {
	db, err := sqlOpen(opts.DataSource, opts.Tracing)
	if err != nil {
		return nil, fmt.Errorf("can't open %s database: %w", driverName, err)
	}

	db.SetMaxOpenConns(opts.MaxOpenConns)
	db.SetMaxIdleConns(opts.MaxIdleConns)
	db.SetConnMaxIdleTime(opts.MaxIdleTime)

	if err = db.Ping(); err != nil {
		return nil, err
	}
	logger.Debugf("connected to [%s] for reads, max open connections: %d, max idle connections: %d, max idle time: %v", driverName, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime)

	return &common.RWDB{
		ReadDB:  db,
		WriteDB: db,
	}, nil
}

func sqlOpen(dataSourceName string, tracing *common2.TracingConfig) (*sql.DB, error) {
	if tracing == nil {
		return sql.Open(driverName, dataSourceName)
	}
	return otelsql.Open(driverName, dataSourceName,
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
	)
}

type Opts struct {
	DataSource      string
	MaxOpenConns    int
	MaxIdleConns    int
	MaxIdleTime     time.Duration
	TablePrefix     string
	TableNameParams []string
	Tracing         *common2.TracingConfig
}
