/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/XSAM/otelsql"
	_ "github.com/jackc/pgx/v5/stdlib"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

var logger = logging.MustGetLogger()

const driverName = "pgx"

// DbProvider returns the connection pool for a set of Opts. Implementations share one
// pool among all callers that pass the same DataSource.
type DbProvider interface {
	Get(Opts) (*common.RWDB, error)
}

// NewDbProvider returns a DbProvider that opens each pool with Open and reports no metrics.
func NewDbProvider() DbProvider { return lazy.NewProviderWithKeyMapper(key, Open) }

// NewDbProviderWithMetrics returns a DbProvider that opens each pool with Open and then
// monitors it for the lifetime of the process: it reports the pool's reachability as the
// database component of the connection-state metrics and exports its pool statistics.
func NewDbProviderWithMetrics(m *connstate.Metrics, p metrics.Provider) DbProvider {
	return lazy.NewProviderWithKeyMapper(key, func(o Opts) (*common.RWDB, error) {
		dbs, err := Open(o)
		if err != nil {
			return nil, err
		}
		label := dataSourceLabel(o.DataSource)
		// Pools are never closed, so the monitor runs until the process exits.
		go monitorDB(context.Background(), dbs.WriteDB, label, defaultPingInterval, m.Tracker("", "", connstate.ComponentDatabase, label), p)
		return dbs, nil
	})
}

func key(o Opts) string { return o.DataSource }

// Open opens a connection pool for opts.DataSource and pings it once; it returns an error
// if the database is unreachable. ReadDB and WriteDB share the same pool.
func Open(opts Opts) (*common.RWDB, error) {
	db, err := sqlOpen(opts.DataSource, opts.Tracing)
	if err != nil {
		return nil, errors.Wrapf(err, "can't open %s database", driverName)
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
